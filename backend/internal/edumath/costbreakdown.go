package edumath

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Cost breakdown (DEV-118). The batch is the problem revision: every machine job
// for that revision (failed, cancelled and retried runs included) and the support
// time attributed to it stay with the batch and are carried by the lessons the
// batch produced. A batch cost is split equally across its accepted lessons, and
// a lesson's share equally across the classes it was assigned to. Rows whose
// denominator is zero are reported as not computable rather than as zero.

const costBreakdownDefinition = "cost_breakdown/1"

var costBreakdownBy = map[string]bool{"tenant": true, "family": true, "class": true, "reuse": true,
	"method": true, "template": true, "model": true, "terminal": true}

// costNotCollected lists strata whose source is partial. Since v97 every stratum
// has a source; rows created before v97 report "not_recorded" for template and
// model, and lessons nobody opened report "not_viewed" for terminal.
var costNotCollected = []map[string]string{
	{"dimension": "template", "reason": "lesson revisions materialized before v97 carry no generator or composer version (key not_recorded)"},
	{"dimension": "model", "reason": "lesson revisions materialized before v97 carry no model record (key not_recorded)"},
	{"dimension": "terminal", "reason": "terminal comes from student reads since v97; a lesson without reads is not_viewed"},
}

// Stratum definitions (DEV-118):
//   - method: the solving method in the confirmed problem revision (batch level, like family);
//   - template: generator@composer_version of each accepted lesson revision;
//   - model: the generative model recorded on the lesson ("none" when the lesson records
//     ai_generated=false), per accepted lesson;
//   - terminal: each accepted lesson's share is split evenly across the distinct
//     (student, terminal) pairs that opened an assignment of that lesson.
var costPerLessonBy = map[string]bool{"class": true, "reuse": true, "template": true, "model": true, "terminal": true}

// CostRow is one stratum of the breakdown.
type CostRow struct {
	Key                   string `json:"key"`
	CostMicro             int64  `json:"costMicro"`
	MachineMicro          int64  `json:"machineMicro"`
	FailedMicro           int64  `json:"failedMicro"`
	SupportMicro          int64  `json:"supportMicro"`
	Jobs                  int64  `json:"jobs"`
	RetriedJobs           int64  `json:"retriedJobs"`
	Accepted              int64  `json:"accepted"`
	ClassDeliveries       int64  `json:"classDeliveries"`
	PerAcceptedMicro      *int64 `json:"perAcceptedMicro"`
	PerClassDeliveryMicro *int64 `json:"perClassDeliveryMicro"`
	Computable            bool   `json:"computable"`
}

// CostBreakdown is the stratified period report.
type CostBreakdown struct {
	Period            string              `json:"period"`
	Timezone          string              `json:"timezone"`
	Definition        string              `json:"definition"`
	By                string              `json:"by"`
	Rows              []*CostRow          `json:"rows"`
	TotalMicro        int64               `json:"totalMicro"`
	UnattributedMicro int64               `json:"supportUnattributedMicro"`
	ExcludedOpenJobs  int64               `json:"excludedOpenJobs"`
	NotCollected      []map[string]string `json:"notCollected"`
}

func perUnit(cost, n int64) *int64 {
	if n <= 0 {
		return nil
	}
	v := cost / n
	return &v
}

// splitEven splits total into n parts that add up exactly (the remainder goes to the first parts).
func splitEven(total int64, n int) []int64 {
	out := make([]int64, n)
	if n == 0 {
		return out
	}
	q, r := total/int64(n), total%int64(n)
	for i := range out {
		out[i] = q
		if int64(i) < r {
			out[i]++
		}
	}
	return out
}

// CostBreakdown computes the stratified report for one tenant and period. Jobs are
// selected exactly as in CostReport (by creation time, settled statuses), so the
// total reconciles with the period report's machine and support lines.
func (s *Service) CostBreakdown(tx *gorm.DB, tenantID, period, by string) (CostBreakdown, error) {
	from, to, err := periodRange(period)
	if err != nil {
		return CostBreakdown{}, err
	}
	out := CostBreakdown{Period: period, Timezone: budgetZone().String(), Definition: costBreakdownDefinition, By: by, NotCollected: costNotCollected}

	type batch struct {
		family, method           string
		machine, failed, support int64
		jobs, retried            int64
		pubs                     []string
	}
	batches := map[string]*batch{}
	get := func(rev string) *batch {
		b := batches[rev]
		if b == nil {
			b = &batch{family: "unknown", method: "not_recorded"}
			batches[rev] = b
		}
		return b
	}
	var jobs []struct {
		ID, Status, Kind, ProblemRevisionID string
		Cost                                int64
	}
	if err := tx.Raw(`SELECT j.id, j.status, j.kind, j.problem_revision_id, COALESCE(SUM(r.cost_micro), 0) AS cost
		FROM edu_math_jobs j LEFT JOIN edu_stage_runs r ON r.job_id = j.id
		WHERE j.tenant_id = ? AND j.created_at >= ? AND j.created_at < ?
		GROUP BY j.id, j.status, j.kind, j.problem_revision_id`, tenantID, from, to).Scan(&jobs).Error; err != nil {
		return out, err
	}
	seen := map[string]bool{}
	for _, j := range jobs {
		switch j.Status {
		case "succeeded", "failed", "cancelled":
		default:
			out.ExcludedOpenJobs++
			continue
		}
		b := get(j.ProblemRevisionID)
		b.machine += j.Cost
		b.jobs++
		if j.Status != "succeeded" {
			b.failed += j.Cost
		}
		k := j.ProblemRevisionID + "|" + j.Kind
		if seen[k] {
			b.retried++
		}
		seen[k] = true
	}
	var fams []struct{ ID, FamilyID, Structured string }
	if err := tx.Raw(`SELECT pr.id, p.family_id, pr.structured FROM edu_problem_revisions pr JOIN edu_problems p ON p.id = pr.problem_id WHERE pr.tenant_id = ?`, tenantID).Scan(&fams).Error; err != nil {
		return out, err
	}
	family, method := map[string]string{}, map[string]string{}
	for _, f := range fams {
		family[f.ID] = f.FamilyID
		var st struct {
			Method string `json:"method"`
		}
		if json.Unmarshal([]byte(f.Structured), &st) == nil && st.Method != "" {
			method[f.ID] = st.Method
		}
	}
	var pubs []struct{ ID, ProblemRevisionID, Generator, ComposerVersion, ModelRef string }
	if err := tx.Raw(`SELECT pub.id, lr.problem_revision_id, lr.generator, lr.composer_version, lr.model_ref FROM edu_publications pub JOIN edu_lesson_revisions lr ON lr.id = pub.lesson_revision_id
		WHERE pub.tenant_id = ? AND pub.created_at >= ? AND pub.created_at < ? ORDER BY pub.id`, tenantID, from, to).Scan(&pubs).Error; err != nil {
		return out, err
	}
	pubRev, pubTemplate, pubModel := map[string]string{}, map[string]string{}, map[string]string{}
	pubIDs := []string{}
	for _, p := range pubs {
		get(p.ProblemRevisionID).pubs = append(get(p.ProblemRevisionID).pubs, p.ID)
		pubRev[p.ID] = p.ProblemRevisionID
		pubIDs = append(pubIDs, p.ID)
		pubTemplate[p.ID], pubModel[p.ID] = "not_recorded", "not_recorded"
		if p.Generator != "" {
			pubTemplate[p.ID] = p.Generator + "@" + p.ComposerVersion
		}
		if p.ModelRef != "" {
			pubModel[p.ID] = p.ModelRef
		}
	}
	for rev, b := range batches {
		if f, ok := family[rev]; ok && f != "" {
			b.family = f
		}
		if m, ok := method[rev]; ok {
			b.method = m
		}
	}
	classes := map[string][]string{}
	if len(pubIDs) > 0 {
		var as []struct{ PublicationID, ClassID string }
		if err := tx.Raw(`SELECT DISTINCT publication_id, class_id FROM edu_assignments WHERE tenant_id = ? AND publication_id IN ? ORDER BY publication_id, class_id`,
			tenantID, pubIDs).Scan(&as).Error; err != nil {
			return out, err
		}
		for _, a := range as {
			classes[a.PublicationID] = append(classes[a.PublicationID], a.ClassID)
		}
	}
	terminals := map[string][]string{} // publication -> one entry per distinct (student, terminal) read
	if by == "terminal" && len(pubIDs) > 0 {
		var vs []struct{ PublicationID, StudentUserID, Terminal string }
		if err := tx.Raw(`SELECT DISTINCT a.publication_id, v.student_user_id, v.terminal FROM edu_assignment_views v JOIN edu_assignments a ON a.id = v.assignment_id
			WHERE v.tenant_id = ? AND a.publication_id IN ? ORDER BY a.publication_id, v.student_user_id, v.terminal`, tenantID, pubIDs).Scan(&vs).Error; err != nil {
			return out, err
		}
		for _, v := range vs {
			terminals[v.PublicationID] = append(terminals[v.PublicationID], "terminal:"+v.Terminal)
		}
	}
	// Support: "publication:<id>" joins the batch, "family:<id>" the family, "class:<id>" the class.
	var sup []struct {
		CostMicro  int64
		SubjectRef string
	}
	if err := tx.Raw(`SELECT cost_micro, subject_ref FROM edu_support_costs WHERE tenant_id = ? AND period = ?`, tenantID, period).Scan(&sup).Error; err != nil {
		return out, err
	}
	famSupport, classSupport := map[string]int64{}, map[string]int64{}
	for _, x := range sup {
		kind, id, _ := strings.Cut(x.SubjectRef, ":")
		switch {
		case kind == "publication" && pubRev[id] != "":
			get(pubRev[id]).support += x.CostMicro
		case kind == "family" && id != "":
			famSupport[id] += x.CostMicro
		case kind == "class" && id != "":
			classSupport[id] += x.CostMicro
		default:
			out.UnattributedMicro += x.CostMicro
		}
	}

	rows := map[string]*CostRow{}
	row := func(key string) *CostRow {
		r := rows[key]
		if r == nil {
			r = &CostRow{Key: key}
			rows[key] = r
		}
		return r
	}
	total := out.UnattributedMicro
	for _, b := range batches {
		total += b.machine + b.support
	}
	for _, v := range famSupport {
		total += v
	}
	for _, v := range classSupport {
		total += v
	}
	out.TotalMicro = total
	revs := make([]string, 0, len(batches))
	for rev := range batches {
		revs = append(revs, rev)
	}
	sort.Strings(revs)

	switch {
	case !costPerLessonBy[by]:
		for _, rev := range revs {
			b := batches[rev]
			key := "tenant"
			switch by {
			case "family":
				key = b.family
			case "method":
				key = b.method
			}
			r := row(key)
			r.MachineMicro += b.machine
			r.FailedMicro += b.failed
			r.SupportMicro += b.support
			r.Jobs += b.jobs
			r.RetriedJobs += b.retried
			r.Accepted += int64(len(b.pubs))
			for _, p := range b.pubs {
				r.ClassDeliveries += int64(len(classes[p]))
			}
		}
		for f, v := range famSupport {
			key := f
			switch by {
			case "tenant":
				key = "tenant"
			case "method":
				key = "support_family_not_split"
			}
			row(key).SupportMicro += v
		}
		for _, v := range classSupport {
			key := "support_class_not_split"
			if by == "tenant" {
				key = "tenant"
			}
			row(key).SupportMicro += v
		}
		if out.UnattributedMicro > 0 {
			key := "support_unattributed"
			if by == "tenant" {
				key = "tenant"
			}
			row(key).SupportMicro += out.UnattributedMicro
		}
	default: // per accepted lesson: class, reuse, template, model, terminal
		for _, rev := range revs {
			b := batches[rev]
			if len(b.pubs) == 0 {
				r := row("no_accepted_lesson")
				r.MachineMicro += b.machine
				r.FailedMicro += b.failed
				r.SupportMicro += b.support
				r.Jobs += b.jobs
				r.RetriedJobs += b.retried
				continue
			}
			machine, support := splitEven(b.machine, len(b.pubs)), splitEven(b.support, len(b.pubs))
			for i, p := range b.pubs {
				cs := classes[p]
				if by == "template" || by == "model" || by == "terminal" {
					keys := []string{pubTemplate[p]}
					switch {
					case by == "model":
						keys = []string{pubModel[p]}
					case by == "terminal" && len(cs) == 0:
						keys = []string{"not_assigned"}
					case by == "terminal" && len(terminals[p]) == 0:
						keys = []string{"not_viewed"}
					case by == "terminal":
						keys = terminals[p]
					}
					m, su := splitEven(machine[i], len(keys)), splitEven(support[i], len(keys))
					for j, k := range keys {
						r := row(k)
						r.MachineMicro += m[j]
						r.SupportMicro += su[j]
						r.Accepted++ // the lesson is accepted in every stratum it is split into, as with classes
						if j == 0 {
							r.ClassDeliveries += int64(len(cs))
						}
					}
					continue
				}
				key := "reused"
				switch len(cs) {
				case 0:
					key = "not_assigned"
				case 1:
					key = "single_class"
				}
				if by == "reuse" || len(cs) == 0 {
					r := row(key)
					r.MachineMicro += machine[i]
					r.SupportMicro += support[i]
					r.Accepted++
					r.ClassDeliveries += int64(len(cs))
					continue
				}
				m, su := splitEven(machine[i], len(cs)), splitEven(support[i], len(cs))
				for j, cl := range cs {
					r := row("class:" + cl)
					r.MachineMicro += m[j]
					r.SupportMicro += su[j]
					r.Accepted++
					r.ClassDeliveries++
				}
			}
		}
		for cl, v := range classSupport {
			if by == "class" {
				row("class:" + cl).SupportMicro += v
			} else {
				row("support_class_not_split").SupportMicro += v
			}
		}
		for _, v := range famSupport {
			row("support_family_not_split").SupportMicro += v
		}
		if out.UnattributedMicro > 0 {
			row("support_unattributed").SupportMicro += out.UnattributedMicro
		}
	}
	for _, r := range rows {
		r.CostMicro = r.MachineMicro + r.SupportMicro
		r.PerAcceptedMicro = perUnit(r.CostMicro, r.Accepted)
		r.PerClassDeliveryMicro = perUnit(r.CostMicro, r.ClassDeliveries)
		r.Computable = r.Accepted > 0
		out.Rows = append(out.Rows, r)
	}
	sort.Slice(out.Rows, func(i, j int) bool { return out.Rows[i].Key < out.Rows[j].Key })
	return out, nil
}

func (h *API) costBreakdown(c *gin.Context) {
	a, tx, ok := h.ledgerAdmin(c)
	if !ok {
		return
	}
	p := c.Query("period")
	if p == "" {
		p, _ = budgetPeriod(h.Svc.now())
	}
	by := c.DefaultQuery("by", "tenant")
	if !periodRe.MatchString(p) || !costBreakdownBy[by] {
		reply(c, 400, gin.H{"notCollected": costNotCollected}, "period YYYY-MM; by tenant, family, class, reuse, method, template, model or terminal")
		return
	}
	r, err := h.Svc.CostBreakdown(tx, a.TenantID, p, by)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	reply(c, 200, r, "ok")
}
