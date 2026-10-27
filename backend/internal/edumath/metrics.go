package edumath

// De-identified metric event pipeline (DEV-116, FR-O01, FR-O02, AC-58).
//
// Metric events are derived on the server from records the workbench already keeps (audit actions for
// teacher steps, the learning tables for student steps, math jobs for job and settlement facts). Every
// event id is a hash of its source row, so a collector run, a replayed window or an overlapping run never
// counts a fact twice. Events carry ids of teaching objects, a per-tenant pseudonym of the acting user
// (HMAC with a salt that is never exported), enumerated attributes and server times. They carry no
// names, free text, answers or problem content. Demo classes live in separate tables and are never
// collected, so staff demonstrations do not enter the metrics. Student events come from the learning
// tables rather than the audit log, and deleting a student's learning data also deletes the student's
// metric events, so a deleted student is never resurrected by a later collection.

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

const (
	metricSchemaVersion = 1
	// metricOverlap re-reads the tail of the previous window; dedupe makes the overlap free.
	metricOverlap = time.Hour
	// metricSmallSample marks a stratum with fewer distinct subjects than this as a small sample.
	metricSmallSample = 5
)

// metricObjRe accepts a workbench object id (prefix, underscore, id body).
var metricObjRe = regexp.MustCompile(`^[a-z]{1,6}_[0-9A-Za-z]{4,40}$`)

// metricTypes is the closed FR-O01 event vocabulary.
var metricTypes = []string{"problem_uploaded", "problem_confirmed", "analysis_completed", "lesson_edited", "review_completed",
	"publication_created", "publication_withdrawn", "assignment_created", "attempt_submitted", "hint_revealed",
	"checkpoint_completed", "independent_unit_submitted", "review_attempted", "feedback_resolved", "artifact_reused",
	"job_completed", "usage_settled"}

// metricAppVersion labels events with the build that collected them (EDU_APP_VERSION, default "dev").
func metricAppVersion() string {
	v := strings.TrimSpace(os.Getenv("EDU_APP_VERSION"))
	if v == "" || len(v) > 40 {
		return "dev"
	}
	return v
}

// MetricsEvery is the scheduled collection interval (EDU_METRICS_EVERY, Go duration; default 1h;
// "0" or "off" disables; at least 5m).
func MetricsEvery() time.Duration {
	v := os.Getenv("EDU_METRICS_EVERY")
	if v == "" {
		return time.Hour
	}
	if v == "off" {
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return time.Hour
	}
	if d > 0 && d < 5*time.Minute {
		d = 5 * time.Minute
	}
	return d
}

func metricPseudonym(salt, user string) string {
	if user == "" {
		return "unknown"
	}
	if strings.HasPrefix(user, "system:") {
		return "system"
	}
	m := hmac.New(sha256.New, []byte(salt))
	m.Write([]byte(user))
	return "p_" + hex.EncodeToString(m.Sum(nil))[:24]
}

func metricID(tenant, source string) string {
	h := sha256.Sum256([]byte(tenant + "|" + source))
	return "emv_" + hex.EncodeToString(h[:])[:32]
}

// metricState loads the tenant's collector state, creating it with a fresh salt when create is set.
func metricState(tx *gorm.DB, tenant string, create bool, now time.Time) (model.EduMetricState, bool, error) {
	var st model.EduMetricState
	if tx.First(&st, "tenant_id = ?", tenant).Error == nil {
		return st, true, nil
	}
	if !create {
		return st, false, nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return st, false, err
	}
	st = model.EduMetricState{TenantID: tenant, Salt: hex.EncodeToString(b), UpdatedAt: now}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&st).Error; err != nil {
		return st, false, err
	}
	if err := tx.First(&st, "tenant_id = ?", tenant).Error; err != nil {
		return st, false, err
	}
	return st, true, nil
}

// metricCollector holds one collection run's lookups.
type metricCollector struct {
	tx       *gorm.DB
	tenant   string
	salt     string
	now      time.Time
	version  string
	inserted int
	family   map[string]string // problem revision id -> family id
	pubRev   map[string]string // publication id -> problem revision id
	assign   map[string]model.EduAssignment
}

func (m *metricCollector) familyOfRevision(rev string) string {
	if rev == "" {
		return ""
	}
	if f, ok := m.family[rev]; ok {
		return f
	}
	var p model.EduProblem
	f := ""
	if m.tx.Model(&model.EduProblem{}).Select("edu_problems.family_id").
		Joins("JOIN edu_problem_revisions r ON r.problem_id = edu_problems.id").
		Where("r.id = ? AND r.tenant_id = ?", rev, m.tenant).Take(&p).Error == nil {
		f = p.FamilyID
	}
	m.family[rev] = f
	return f
}

func (m *metricCollector) familyOfProblem(problem string) string {
	var p model.EduProblem
	if m.tx.Select("family_id").Take(&p, "id = ? AND tenant_id = ?", problem, m.tenant).Error != nil {
		return ""
	}
	return p.FamilyID
}

func (m *metricCollector) familyOfPublication(pub string) string {
	if pub == "" {
		return ""
	}
	rev, ok := m.pubRev[pub]
	if !ok {
		var l model.EduLessonRevision
		if m.tx.Model(&model.EduLessonRevision{}).Select("edu_lesson_revisions.problem_revision_id").
			Joins("JOIN edu_publications p ON p.lesson_revision_id = edu_lesson_revisions.id").
			Where("p.id = ? AND p.tenant_id = ?", pub, m.tenant).Take(&l).Error == nil {
			rev = l.ProblemRevisionID
		}
		m.pubRev[pub] = rev
	}
	return m.familyOfRevision(rev)
}

func (m *metricCollector) assignment(id string) model.EduAssignment {
	if as, ok := m.assign[id]; ok {
		return as
	}
	var as model.EduAssignment
	m.tx.Take(&as, "id = ? AND tenant_id = ?", id, m.tenant)
	m.assign[id] = as
	return as
}

func (m *metricCollector) emit(ev model.EduMetricEvent, attrs map[string]any) error {
	ev.ID, ev.TenantID = metricID(m.tenant, ev.Source), m.tenant
	if len(attrs) > 0 {
		b, _ := json.Marshal(attrs)
		ev.Attrs = string(b)
	}
	ev.CollectedAt, ev.AppVersion, ev.SchemaVersion = m.now, m.version, metricSchemaVersion
	res := m.tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ev)
	if res.Error != nil {
		return res.Error
	}
	m.inserted += int(res.RowsAffected)
	return nil
}

func roleOf(user, fallback string) string {
	if user == "" || strings.HasPrefix(user, "system:") {
		return "system"
	}
	return fallback
}

// attemptMode classifies an attempt for AC-58: missing (no result recorded), after_hints (hints or help
// used before it), first_try (the student's first valid answer, unaided) or retry (any later unaided
// attempt). Independence itself is kept as its own attribute.
func attemptMode(at model.EduAttempt) string {
	switch {
	case at.Result == "":
		return "missing"
	case !at.Independent:
		return "after_hints"
	case at.FirstValid:
		return "first_try"
	}
	return "retry"
}

var auditMetricTypes = map[string]string{"problem.create": "problem_uploaded", "problem.confirm": "problem_confirmed", "lesson.edit": "lesson_edited",
	"lesson.publish": "publication_created", "incident.open.blocking": "publication_withdrawn"}

// collect derives the events whose source time lies in (from, to].
//
// The SQL bounds are widened by a day and the exact window is applied in Go, because some drivers store
// times with the writer's zone offset and compare them as text; the exact check uses real instants.
func (m *metricCollector) collect(from, to time.Time) error {
	in := func(t time.Time) bool { return t.After(from) && !t.After(to) }
	lo, hi := from, to.Add(24*time.Hour)
	if !from.IsZero() {
		lo = from.Add(-24 * time.Hour)
	}
	// Teacher steps from the audit log. Only the action, actor and subject id are read; details may hold
	// free text (reasons) and are never copied, except a publication id that is validated as an id.
	var aus []model.EduAuditEvent
	if err := m.tx.Where("tenant_id = ? AND created_at > ? AND created_at <= ? AND (action IN ? OR action LIKE ?)", m.tenant, lo, hi,
		[]string{"problem.create", "problem.confirm", "lesson.edit", "lesson.publish", "incident.open.blocking"}, "lesson.review.%").Find(&aus).Error; err != nil {
		return err
	}
	for _, au := range aus {
		if !in(au.CreatedAt) {
			continue
		}
		ev := model.EduMetricEvent{Source: "audit:" + au.ID, Subject: metricPseudonym(m.salt, au.ActorID), Role: roleOf(au.ActorID, "teacher"), ObjectID: au.Subject, OccurredAt: au.CreatedAt}
		var attrs map[string]any
		switch {
		case strings.HasPrefix(au.Action, "lesson.review."):
			ev.Type, attrs = "review_completed", map[string]any{"decision": strings.TrimPrefix(au.Action, "lesson.review.")}
			var l model.EduLessonRevision
			if m.tx.Select("problem_revision_id").Take(&l, "id = ? AND tenant_id = ?", au.Subject, m.tenant).Error == nil {
				ev.FamilyID = m.familyOfRevision(l.ProblemRevisionID)
			}
		default:
			ev.Type = auditMetricTypes[au.Action]
		}
		switch ev.Type {
		case "problem_uploaded":
			ev.FamilyID = m.familyOfProblem(au.Subject)
		case "problem_confirmed":
			ev.FamilyID = m.familyOfRevision(au.Subject)
		case "publication_created":
			ev.PublicationID, ev.FamilyID = au.Subject, m.familyOfPublication(au.Subject)
		case "publication_withdrawn":
			if pub := strings.TrimSpace(au.Detail); metricObjRe.MatchString(pub) {
				ev.PublicationID, ev.FamilyID = pub, m.familyOfPublication(pub)
			}
		}
		if err := m.emit(ev, attrs); err != nil {
			return err
		}
	}
	// Assignments, and reuse of a publication already assigned in another class.
	var ass []model.EduAssignment
	if err := m.tx.Where("tenant_id = ? AND created_at > ? AND created_at <= ?", m.tenant, lo, hi).Find(&ass).Error; err != nil {
		return err
	}
	for _, as := range ass {
		if !in(as.CreatedAt) {
			continue
		}
		m.assign[as.ID] = as
		var prior int64
		m.tx.Model(&model.EduAssignment{}).Where("tenant_id = ? AND publication_id = ? AND class_id <> ? AND (created_at < ? OR (created_at = ? AND id < ?))",
			m.tenant, as.PublicationID, as.ClassID, as.CreatedAt, as.CreatedAt, as.ID).Count(&prior)
		ev := model.EduMetricEvent{Type: "assignment_created", Source: "assignment:" + as.ID, Subject: metricPseudonym(m.salt, as.CreatedBy), Role: roleOf(as.CreatedBy, "teacher"),
			ObjectID: as.ID, ClassID: as.ClassID, AssignmentID: as.ID, PublicationID: as.PublicationID, FamilyID: m.familyOfPublication(as.PublicationID), OccurredAt: as.CreatedAt}
		if err := m.emit(ev, map[string]any{"reused": prior > 0}); err != nil {
			return err
		}
		if prior > 0 {
			ev.Type, ev.Source = "artifact_reused", "reuse:"+as.ID
			if err := m.emit(ev, nil); err != nil {
				return err
			}
		}
	}
	student := func(typ, source, user, assignment, object string, at time.Time) model.EduMetricEvent {
		as := m.assignment(assignment)
		return model.EduMetricEvent{Type: typ, Source: source, Subject: metricPseudonym(m.salt, user), Role: "student", ObjectID: object, ClassID: as.ClassID,
			AssignmentID: assignment, PublicationID: as.PublicationID, FamilyID: m.familyOfPublication(as.PublicationID), OccurredAt: at}
	}
	var ats []model.EduAttempt
	if err := m.tx.Where("tenant_id = ? AND created_at > ? AND created_at <= ?", m.tenant, lo, hi).Find(&ats).Error; err != nil {
		return err
	}
	for _, at := range ats {
		if !in(at.CreatedAt) {
			continue
		}
		typ := "attempt_submitted"
		switch {
		case at.ReviewItemID != "":
			typ = "review_attempted"
		case at.Item == "check":
			typ = "checkpoint_completed"
		}
		result := at.Result
		if result == "" {
			result = "missing"
		}
		if err := m.emit(student(typ, "attempt:"+at.ID, at.StudentUserID, at.AssignmentID, at.ID, at.CreatedAt),
			map[string]any{"item": at.Item, "kind": at.Kind, "result": result, "mode": attemptMode(at), "independent": at.Independent, "hintLevel": at.HintLevel}); err != nil {
			return err
		}
	}
	var hs []model.EduHintUnlock
	if err := m.tx.Where("tenant_id = ? AND created_at > ? AND created_at <= ?", m.tenant, lo, hi).Find(&hs).Error; err != nil {
		return err
	}
	for _, hu := range hs {
		if !in(hu.CreatedAt) {
			continue
		}
		if err := m.emit(student("hint_revealed", "hint:"+hu.ID, hu.StudentUserID, hu.AssignmentID, hu.ID, hu.CreatedAt), map[string]any{"level": hu.Level}); err != nil {
			return err
		}
	}
	var sts []model.EduSelfTest
	if err := m.tx.Where("tenant_id = ? AND state = ? AND submitted_at > ? AND submitted_at <= ?", m.tenant, "submitted", lo, hi).Find(&sts).Error; err != nil {
		return err
	}
	for _, st := range sts {
		if !in(*st.SubmittedAt) {
			continue
		}
		if err := m.emit(student("independent_unit_submitted", "selftest:"+st.ID, st.StudentUserID, st.AssignmentID, st.ID, *st.SubmittedAt), nil); err != nil {
			return err
		}
	}
	var hr []model.EduHelpRequest
	if err := m.tx.Where("tenant_id = ? AND state = ? AND updated_at > ? AND updated_at <= ?", m.tenant, "answered", lo, hi).Find(&hr).Error; err != nil {
		return err
	}
	for _, r := range hr {
		if !in(r.UpdatedAt) {
			continue
		}
		if err := m.emit(student("feedback_resolved", "help:"+r.ID, r.StudentUserID, r.AssignmentID, r.ID, r.UpdatedAt), map[string]any{"kind": r.Kind}); err != nil {
			return err
		}
	}
	// Jobs: terminal outcomes and settlement. Unknown provider results stay "reconciliation_required"
	// and are not terminal, so they are not counted as completed.
	var js []model.EduMathJob
	if err := m.tx.Where("tenant_id = ? AND updated_at > ? AND updated_at <= ? AND (status IN ? OR settlement = ?)", m.tenant, lo, hi,
		[]string{"succeeded", "failed", "cancelled"}, "settled").Find(&js).Error; err != nil {
		return err
	}
	for _, j := range js {
		if !in(j.UpdatedAt) {
			continue
		}
		base := model.EduMetricEvent{Subject: metricPseudonym(m.salt, j.CreatedBy), Role: roleOf(j.CreatedBy, "teacher"), ObjectID: j.ID,
			FamilyID: m.familyOfRevision(j.ProblemRevisionID), OccurredAt: j.UpdatedAt}
		if j.Status == "succeeded" || j.Status == "failed" || j.Status == "cancelled" {
			ev := base
			ev.Type, ev.Source = "job_completed", "job:"+j.ID
			if err := m.emit(ev, map[string]any{"jobKind": j.Kind, "status": j.Status}); err != nil {
				return err
			}
			if j.Kind == "analyze" && j.Status == "succeeded" {
				ev.Type, ev.Source = "analysis_completed", "analysis:"+j.ID
				if err := m.emit(ev, nil); err != nil {
					return err
				}
			}
		}
		if j.Settlement == "settled" {
			ev := base
			ev.Type, ev.Source = "usage_settled", "settle:"+j.ID
			if err := m.emit(ev, map[string]any{"jobKind": j.Kind}); err != nil {
				return err
			}
		}
	}
	return nil
}

// collectMetricsTx runs one incremental collection for a tenant: from the previous watermark minus the
// overlap up to now. A full replay (from the zero time) is also safe because events are deduplicated.
func (s *Service) collectMetricsTx(tx *gorm.DB, tenant string, replay bool) (int, error) {
	now := s.now()
	st, _, err := metricState(tx, tenant, true, now)
	if err != nil {
		return 0, err
	}
	from := time.Time{}
	if !replay && !st.CollectedUntil.IsZero() {
		from = st.CollectedUntil.Add(-metricOverlap)
	}
	m := &metricCollector{tx: tx, tenant: tenant, salt: st.Salt, now: now, version: metricAppVersion(),
		family: map[string]string{}, pubRev: map[string]string{}, assign: map[string]model.EduAssignment{}}
	if err := m.collect(from, now); err != nil {
		return m.inserted, err
	}
	until := now
	if st.CollectedUntil.After(until) {
		until = st.CollectedUntil
	}
	err = tx.Model(&model.EduMetricState{}).Where("tenant_id = ?", tenant).Updates(map[string]any{"collected_until": until, "updated_at": now}).Error
	return m.inserted, err
}

// MetricsAll collects metric events for every tenant with recorded activity, each in its own tenant
// transaction.
func (s *Service) MetricsAll(ctx context.Context) (tenants, inserted int, err error) {
	var ids []string
	if err = s.DB.Model(&model.EduAuditEvent{}).Distinct().Pluck("tenant_id", &ids).Error; err != nil {
		return 0, 0, err
	}
	for _, tenant := range ids {
		if ctx.Err() != nil {
			return tenants, inserted, ctx.Err()
		}
		var n int
		e := flow.TenantTx(s.DB, tenant, func(tx *gorm.DB) error {
			var err error
			n, err = s.collectMetricsTx(tx, tenant, false)
			return err
		})
		if e != nil {
			slog.Warn("edumath metric collection failed", "tenant", tenant, "error", e)
			continue
		}
		tenants++
		inserted += n
	}
	return tenants, inserted, nil
}

// deleteStudentMetrics removes a student's metric events (all, or one class's) together with the
// learning data they were derived from.
func deleteStudentMetrics(tx *gorm.DB, tenant, student, classID string, dry bool) (int, error) {
	st, ok, err := metricState(tx, tenant, false, time.Time{})
	if err != nil || !ok {
		return 0, err
	}
	scope := func(q *gorm.DB) *gorm.DB {
		q = q.Where("tenant_id = ? AND role = ? AND subject = ?", tenant, "student", metricPseudonym(st.Salt, student))
		if classID != "" {
			q = q.Where("class_id = ?", classID)
		}
		return q
	}
	var n int64
	if err := scope(tx.Model(&model.EduMetricEvent{})).Count(&n).Error; err != nil || n == 0 || dry {
		return int(n), err
	}
	return int(n), scope(tx.Session(&gorm.Session{NewDB: true})).Delete(&model.EduMetricEvent{}).Error
}

func (h *API) registerMetrics(g *gin.RouterGroup) {
	g.GET("/metrics", h.metricsSummary)
	g.POST("/metrics/collect", h.collectMetrics)
}

func (h *API) collectMetrics(c *gin.Context) {
	a, tx, ok := h.orgAdmin(c)
	if !ok {
		return
	}
	var in struct {
		Replay bool `json:"replay"`
	}
	_ = c.ShouldBindJSON(&in)
	n, err := h.Svc.collectMetricsTx(tx, a.TenantID, in.Replay)
	if err != nil {
		h.fail(c, err)
		return
	}
	var total int64
	tx.Model(&model.EduMetricEvent{}).Where("tenant_id = ?", a.TenantID).Count(&total)
	h.Svc.audit(tx, a, "metrics.collect", a.TenantID, "")
	reply(c, 200, gin.H{"inserted": n, "total": total, "replay": in.Replay}, "ok")
}

// metricsSummary reports deduplicated counts per event type for the tenant, optionally narrowed to a
// class, family and time range, with distinct subjects, the attempt mode split (AC-58), small sample
// flags and the dimensions that are missing (FR-O02).
func (h *API) metricsSummary(c *gin.Context) {
	a, tx, ok := h.orgAdmin(c)
	if !ok {
		return
	}
	q := tx.Model(&model.EduMetricEvent{}).Where("tenant_id = ?", a.TenantID)
	for _, f := range []struct{ param, col string }{{"classId", "class_id"}, {"familyId", "family_id"}, {"publicationId", "publication_id"}} {
		if v := c.Query(f.param); v != "" {
			if !metricObjRe.MatchString(v) {
				h.fail(c, appErr(400, "VALIDATION_FAILED", f.param+" is not an id", "fix_input"))
				return
			}
			q = q.Where(f.col+" = ?", v)
		}
	}
	for _, f := range []struct{ param, op string }{{"from", ">="}, {"to", "<"}} {
		if v := c.Query(f.param); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				h.fail(c, appErr(400, "VALIDATION_FAILED", f.param+" must be an RFC 3339 time", "fix_input"))
				return
			}
			q = q.Where("occurred_at "+f.op+" ?", t)
		}
	}
	var evs []model.EduMetricEvent
	if err := q.Select("type", "subject", "role", "class_id", "family_id", "attrs", "terminal").Find(&evs).Error; err != nil {
		h.fail(c, err)
		return
	}
	type agg struct {
		Events      int            `json:"events"`
		Subjects    int            `json:"subjects"`
		SmallSample bool           `json:"smallSample"`
		Modes       map[string]int `json:"modes,omitempty"`
		Independent int            `json:"independent"`
		Missing     map[string]int `json:"missing"`
		subjects    map[string]bool
	}
	byType := map[string]*agg{}
	for _, t := range metricTypes {
		byType[t] = &agg{Missing: map[string]int{}, subjects: map[string]bool{}}
	}
	reused := map[bool]int{}
	for _, e := range evs {
		g := byType[e.Type]
		if g == nil {
			continue
		}
		g.Events++
		g.subjects[e.Subject] = true
		if e.Terminal == "" {
			g.Missing["terminal"]++
		}
		if e.FamilyID == "" {
			g.Missing["family"]++
		}
		var at map[string]any
		_ = json.Unmarshal([]byte(e.Attrs), &at)
		switch e.Type {
		case "attempt_submitted", "checkpoint_completed", "review_attempted":
			if g.Modes == nil {
				g.Modes = map[string]int{}
			}
			mode, _ := at["mode"].(string)
			g.Modes[mode]++
			if ind, _ := at["independent"].(bool); ind {
				g.Independent++
			}
			if at["result"] == "missing" {
				g.Missing["result"]++
			}
		case "assignment_created":
			r, _ := at["reused"].(bool)
			reused[r]++
		}
	}
	for _, g := range byType {
		g.Subjects = len(g.subjects)
		g.SmallSample = g.Events > 0 && g.Subjects < metricSmallSample
	}
	var st model.EduMetricState
	tx.First(&st, "tenant_id = ?", a.TenantID)
	types := append([]string(nil), metricTypes...)
	sort.Strings(types)
	reply(c, 200, gin.H{"types": byType, "order": types, "assignments": gin.H{"new": reused[false], "reused": reused[true]},
		"collectedUntil": st.CollectedUntil, "schemaVersion": metricSchemaVersion,
		"notCollected": []string{"terminal", "method", "model_version", "template_version"}, "demoExcluded": true}, "ok")
}
