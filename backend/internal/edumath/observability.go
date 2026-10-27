package edumath

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"yingce/backend/internal/model"
)

// Observability (DEV-121). Two read-only views over facts the module already
// writes; nothing here creates tables or calls the math service:
//   - GET /edu/observability: queue, stuck and open jobs, failures by kind and
//     error code, unknown cost, unsettled reservations and submission volume,
//     each with an alert rule (firing | ok | insufficient_data).
//   - GET /edu/jobs/:id/trace and /edu/ops/jobs/:id/trace: one job's chain of
//     platform outbox events, service events, stage settlements and ledger
//     anomalies with the revisions it produced, metadata only.
// Who may look: an org admin of the tenant, or an operator holding an active
// "jobs" support grant (DEV-108; every operator read is audited). Teachers,
// reviewers and students may not. Inputs, outputs, error bodies, price books and
// answers never appear.
// Not collected (listed in the reply): service availability against the 99.5%
// pilot target needs an external probe; reading volume has no view events;
// request level trace ids, host resources and alert delivery belong to the
// deployment (07).

const (
	observabilityDefinition = "observability/1"
	stuckAfter              = 30 * time.Minute
	unsettledAfter          = 10 * time.Minute
	queueAgeAlert           = 15 * time.Minute
	failureRateAlert        = 0.2
	failureRateMinJobs      = 5
)

var observabilityNotCollected = []map[string]string{
	{"dimension": "availability", "reason": "the 99.5% pilot availability target needs an external probe; the module cannot measure its own downtime"},
	{"dimension": "reading", "reason": "no view events are collected (DEV-116 records submissions, not reads)"},
	{"dimension": "request_trace", "reason": "request level trace ids are not propagated; the job id is the trace key"},
	{"dimension": "resources", "reason": "host CPU, memory and disk belong to the deployment monitoring (07)"},
	{"dimension": "alert_delivery", "reason": "alerts are computed on read; paging and notification belong to the deployment (07)"},
}

var openJobStatuses = []string{"queued", "running"}
var terminalJobStatuses = []string{"succeeded", "failed", "cancelled"}

func (h *API) registerObservability(g *gin.RouterGroup) {
	g.GET("/observability", h.observability)
	g.GET("/jobs/:id/trace", h.jobTrace)
	g.GET("/ops/observability", h.opsObservability)
	g.GET("/ops/jobs/:id/trace", h.opsJobTrace)
}

func observabilityHours(c *gin.Context) (int, bool) {
	raw := c.DefaultQuery("hours", "24")
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 168 {
		return 0, false
	}
	return n, true
}

type obsKind struct {
	Kind        string   `json:"kind"`
	Created     int64    `json:"created"`
	Succeeded   int64    `json:"succeeded"`
	Failed      int64    `json:"failed"`
	Cancelled   int64    `json:"cancelled"`
	Open        int64    `json:"open"`
	Reconcile   int64    `json:"reconciliationRequired"`
	FailureRate *float64 `json:"failureRate"`
	P50Seconds  *int64   `json:"p50Seconds"`
	P95Seconds  *int64   `json:"p95Seconds"`
}

func percentile(sorted []int64, p float64) *int64 {
	if len(sorted) == 0 {
		return nil
	}
	i := int(p*float64(len(sorted)) + 0.999999)
	if i < 1 {
		i = 1
	}
	if i > len(sorted) {
		i = len(sorted)
	}
	v := sorted[i-1]
	return &v
}

type obsAlert struct {
	ID        string  `json:"id"`
	State     string  `json:"state"` // firing | ok | insufficient_data
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Rule      string  `json:"rule"`
}

// buildObservability computes the tenant view at now over the last hours.
func buildObservability(tx *gorm.DB, tenant string, now time.Time, hours int) (gin.H, error) {
	from := now.Add(-time.Duration(hours) * time.Hour)
	var jobs []model.EduMathJob
	if err := tx.Select("id", "kind", "status", "stage", "error_code", "settlement", "created_at", "updated_at").
		Where("tenant_id = ? AND created_at >= ?", tenant, from).Find(&jobs).Error; err != nil {
		return nil, err
	}
	kinds := map[string]*obsKind{}
	durations := map[string][]int64{}
	errorCodes := map[string]int64{}
	var failed, terminal int64
	for _, j := range jobs {
		k := kinds[j.Kind]
		if k == nil {
			k = &obsKind{Kind: j.Kind}
			kinds[j.Kind] = k
		}
		k.Created++
		switch j.Status {
		case "succeeded":
			k.Succeeded++
		case "failed":
			k.Failed++
			code := j.ErrorCode
			if code == "" {
				code = "unspecified"
			}
			errorCodes[code]++
		case "cancelled":
			k.Cancelled++
		case "reconciliation_required":
			k.Reconcile++
		default:
			k.Open++
		}
		if j.Status == "succeeded" || j.Status == "failed" {
			durations[j.Kind] = append(durations[j.Kind], int64(j.UpdatedAt.Sub(j.CreatedAt)/time.Second))
		}
	}
	kindRows := []*obsKind{}
	for name, k := range kinds {
		if n := k.Succeeded + k.Failed; n > 0 {
			r := float64(k.Failed) / float64(n)
			k.FailureRate = &r
		}
		d := durations[name]
		sort.Slice(d, func(a, b int) bool { return d[a] < d[b] })
		k.P50Seconds, k.P95Seconds = percentile(d, 0.5), percentile(d, 0.95)
		failed += k.Failed
		terminal += k.Succeeded + k.Failed
		kindRows = append(kindRows, k)
	}
	sort.Slice(kindRows, func(a, b int) bool { return kindRows[a].Kind < kindRows[b].Kind })
	codes := []gin.H{}
	for code, n := range errorCodes {
		codes = append(codes, gin.H{"errorCode": code, "count": n})
	}
	sort.Slice(codes, func(a, b int) bool {
		if codes[a]["count"].(int64) != codes[b]["count"].(int64) {
			return codes[a]["count"].(int64) > codes[b]["count"].(int64)
		}
		return codes[a]["errorCode"].(string) < codes[b]["errorCode"].(string)
	})

	// Current state, regardless of the window.
	var open []model.EduMathJob
	if err := tx.Select("id", "kind", "status", "stage", "created_at", "updated_at").
		Where("tenant_id = ? AND status IN ?", tenant, openJobStatuses).Order("created_at").Find(&open).Error; err != nil {
		return nil, err
	}
	var queued, running int64
	var oldestQueued *int64
	stuck := []gin.H{}
	for _, j := range open {
		if j.Status == "queued" {
			queued++
			if oldestQueued == nil {
				v := int64(now.Sub(j.CreatedAt) / time.Second)
				oldestQueued = &v
			}
		} else {
			running++
		}
		if now.Sub(j.UpdatedAt) > stuckAfter && len(stuck) < 20 {
			stuck = append(stuck, gin.H{"id": j.ID, "kind": j.Kind, "status": j.Status, "stage": j.Stage, "silentSeconds": int64(now.Sub(j.UpdatedAt) / time.Second)})
		}
	}
	var stuckTotal int64
	if err := tx.Model(&model.EduMathJob{}).Where("tenant_id = ? AND status IN ? AND updated_at < ?", tenant, openJobStatuses, now.Add(-stuckAfter)).Count(&stuckTotal).Error; err != nil {
		return nil, err
	}
	var reconcile, heldUnknown, unsettled, anomalies int64
	if err := tx.Model(&model.EduMathJob{}).Where("tenant_id = ? AND status = ?", tenant, "reconciliation_required").Count(&reconcile).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&model.EduMathJob{}).Where("tenant_id = ? AND settlement = ?", tenant, "held_unknown").Count(&heldUnknown).Error; err != nil {
		return nil, err
	}
	// A finished job whose reservation is still open holds budget nobody will use: a resource leak.
	if err := tx.Model(&model.EduMathJob{}).Where("tenant_id = ? AND status IN ? AND settlement = ? AND updated_at < ?", tenant, terminalJobStatuses, "open", now.Add(-unsettledAfter)).
		Count(&unsettled).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&model.EduLedgerAnomaly{}).Where("tenant_id = ? AND status <> ?", tenant, "resolved").Count(&anomalies).Error; err != nil {
		return nil, err
	}

	// Submissions (students) in the window; generation is the job table above.
	var attempts, selfTests, helps int64
	if err := tx.Model(&model.EduAttempt{}).Where("tenant_id = ? AND created_at >= ?", tenant, from).Count(&attempts).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&model.EduSelfTest{}).Where("tenant_id = ? AND submitted_at >= ?", tenant, from).Count(&selfTests).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&model.EduHelpRequest{}).Where("tenant_id = ? AND created_at >= ?", tenant, from).Count(&helps).Error; err != nil {
		return nil, err
	}

	alerts := []obsAlert{
		{ID: "stuck_jobs", Value: float64(stuckTotal), Threshold: 0, Rule: "open jobs silent for more than 30 minutes"},
		{ID: "queue_age", Threshold: queueAgeAlert.Seconds(), Rule: "oldest queued job waiting more than 15 minutes"},
		{ID: "reconciliation_required", Value: float64(reconcile), Threshold: 0, Rule: "jobs whose service outcome is unknown and need reconciliation"},
		{ID: "unknown_cost", Value: float64(heldUnknown), Threshold: 0, Rule: "reservations held because the provider charge is unknown"},
		{ID: "unsettled_reservations", Value: float64(unsettled), Threshold: 0, Rule: "finished jobs whose reservation is still open after 10 minutes"},
		{ID: "ledger_anomalies", Value: float64(anomalies), Threshold: 0, Rule: "unresolved ledger anomalies"},
		{ID: "failure_rate", Threshold: failureRateAlert, Rule: "failed share of finished jobs in the window above 20% (at least 5 finished jobs)"},
	}
	for i := range alerts {
		a := &alerts[i]
		switch a.ID {
		case "queue_age":
			if oldestQueued != nil {
				a.Value = float64(*oldestQueued)
			}
		case "failure_rate":
			if terminal < failureRateMinJobs {
				a.State = "insufficient_data"
				if terminal > 0 {
					a.Value = float64(failed) / float64(terminal)
				}
				continue
			}
			a.Value = float64(failed) / float64(terminal)
		}
		if a.Value > a.Threshold {
			a.State = "firing"
		} else {
			a.State = "ok"
		}
	}
	firing := 0
	for _, a := range alerts {
		if a.State == "firing" {
			firing++
		}
	}
	return gin.H{
		"definition": observabilityDefinition, "generatedAt": now, "windowHours": hours, "from": from,
		"generation":   gin.H{"byKind": kindRows, "errorCodes": codes, "finished": terminal, "failed": failed},
		"submission":   gin.H{"attempts": attempts, "selfTests": selfTests, "helpRequests": helps},
		"reading":      gin.H{"collected": false},
		"queue":        gin.H{"queued": queued, "running": running, "oldestQueuedSeconds": oldestQueued, "stuck": stuck, "stuckTotal": stuckTotal},
		"cost":         gin.H{"reconciliationRequired": reconcile, "heldUnknown": heldUnknown, "unsettledReservations": unsettled, "openLedgerAnomalies": anomalies},
		"availability": gin.H{"target": 0.995, "measured": nil, "reason": "external probe required"},
		"alerts":       alerts, "firing": firing,
		"notCollected": observabilityNotCollected,
	}, nil
}

func (h *API) observability(c *gin.Context) {
	a, tx, ok := h.orgAdmin(c)
	if !ok {
		return
	}
	hours, good := observabilityHours(c)
	if !good {
		h.fail(c, appErr(400, "VALIDATION_FAILED", "hours must be 1 to 168", "fix_input"))
		return
	}
	out, err := buildObservability(tx, a.TenantID, h.Svc.now(), hours)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	reply(c, http.StatusOK, out, "ok")
}

func (h *API) opsObservability(c *gin.Context) {
	a, tx, gr, ok := h.ops(c)
	if !ok {
		return
	}
	if gr.Scope != "jobs" {
		h.fail(c, ErrOpsGrant)
		return
	}
	hours, good := observabilityHours(c)
	if !good {
		h.fail(c, appErr(400, "VALIDATION_FAILED", "hours must be 1 to 168", "fix_input"))
		return
	}
	out, err := buildObservability(tx, a.TenantID, h.Svc.now(), hours)
	if err != nil {
		h.fail(c, err)
		return
	}
	h.Svc.audit(tx, a, "ops.read", gr.ID, "GET /api/edu/ops/observability | "+gr.Purpose)
	c.Header("Cache-Control", "private, no-store")
	reply(c, http.StatusOK, out, "ok")
}

// buildTrace returns one job's metadata chain, or ErrNotFound outside the tenant.
func buildTrace(tx *gorm.DB, tenant, jobID string) (gin.H, error) {
	var j model.EduMathJob
	if err := tx.First(&j, "id = ? AND tenant_id = ?", jobID, tenant).Error; err != nil {
		return nil, ErrNotFound
	}
	type step struct {
		At     time.Time `json:"at"`
		Source string    `json:"source"` // platform | service | stage | ledger
		Type   string    `json:"type"`
		Status string    `json:"status,omitempty"`
		Detail gin.H     `json:"detail,omitempty"`
	}
	steps := []step{}
	var evs []model.EduJobEvent
	if err := tx.Where("tenant_id = ? AND job_id = ?", tenant, j.ID).Order("seq").Find(&evs).Error; err != nil {
		return nil, err
	}
	for _, e := range evs {
		steps = append(steps, step{At: e.CreatedAt, Source: "platform", Type: e.Type, Status: e.Status, Detail: gin.H{"seq": e.Seq, "settlement": e.Settlement, "serviceSeq": e.ServiceSeq}})
	}
	var mes []model.EduMathEvent
	if err := tx.Where("tenant_id = ? AND job_id = ?", tenant, j.ID).Order("seq").Find(&mes).Error; err != nil {
		return nil, err
	}
	for _, e := range mes {
		steps = append(steps, step{At: e.ReceivedAt, Source: "service", Type: "service.event", Status: e.Status, Detail: gin.H{"seq": e.Seq, "result": e.Result}})
	}
	var stages []model.EduStageRun
	if err := tx.Where("tenant_id = ? AND job_id = ?", tenant, j.ID).Order("created_at").Find(&stages).Error; err != nil {
		return nil, err
	}
	var costMicro, computeMS int64
	for _, s := range stages {
		costMicro += s.CostMicro
		computeMS += s.ComputeMS
		steps = append(steps, step{At: s.CreatedAt, Source: "stage", Type: "stage." + s.Stage, Status: s.Status, Detail: gin.H{"computeMs": s.ComputeMS, "costMicro": s.CostMicro, "settled": s.SettledAt != nil}})
	}
	var ans []model.EduLedgerAnomaly
	if err := tx.Where("tenant_id = ? AND job_id = ?", tenant, j.ID).Order("detected_at").Find(&ans).Error; err != nil {
		return nil, err
	}
	for _, x := range ans {
		steps = append(steps, step{At: x.DetectedAt, Source: "ledger", Type: "anomaly." + x.Kind, Status: x.Status})
	}
	sort.SliceStable(steps, func(a, b int) bool { return steps[a].At.Before(steps[b].At) })
	revs := gin.H{"problemRevisionId": j.ProblemRevisionID}
	var sr model.EduSolutionRevision
	if tx.Select("id", "math_state", "solution_status").Where("tenant_id = ? AND job_id = ?", tenant, j.ID).Limit(1).Find(&sr); sr.ID != "" {
		revs["solutionRevisionId"], revs["mathState"], revs["solutionStatus"] = sr.ID, sr.MathState, sr.SolutionStatus
	}
	var lr model.EduLessonRevision
	if tx.Select("id", "state").Where("tenant_id = ? AND job_id = ?", tenant, j.ID).Limit(1).Find(&lr); lr.ID != "" {
		revs["lessonRevisionId"], revs["lessonState"] = lr.ID, lr.State
	}
	return gin.H{
		"definition": observabilityDefinition, "traceId": j.ID,
		"job": gin.H{"id": j.ID, "kind": j.Kind, "status": j.Status, "stage": j.Stage, "sourceJobId": j.SourceJobID, "serviceJobId": j.ServiceJobID,
			"operationId": j.OperationID, "errorCode": j.ErrorCode, "settlement": j.Settlement, "priceVersion": j.PriceVersion, "budgetMicro": j.BudgetMicro,
			"cancelRequested": j.CancelRequested, "createdAt": j.CreatedAt, "updatedAt": j.UpdatedAt},
		"revisions": revs,
		"usage":     gin.H{"costMicro": costMicro, "computeMs": computeMS, "stages": len(stages)},
		"steps":     steps,
	}, nil
}

func (h *API) jobTrace(c *gin.Context) {
	a, tx, ok := h.orgAdmin(c)
	if !ok {
		return
	}
	out, err := buildTrace(tx, a.TenantID, c.Param("id"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	reply(c, http.StatusOK, out, "ok")
}

func (h *API) opsJobTrace(c *gin.Context) {
	a, tx, gr, ok := h.ops(c)
	if !ok {
		return
	}
	if gr.Scope != "jobs" {
		h.fail(c, ErrOpsGrant)
		return
	}
	out, err := buildTrace(tx, a.TenantID, c.Param("id"))
	if err != nil {
		h.fail(c, err)
		return
	}
	h.Svc.audit(tx, a, "ops.read", gr.ID, "GET /api/edu/ops/jobs/:id/trace | "+gr.Purpose)
	c.Header("Cache-Control", "private, no-store")
	reply(c, http.StatusOK, out, "ok")
}
