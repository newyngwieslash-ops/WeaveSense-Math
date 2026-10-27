package edumath

// DEV-122: backup restore drill support. A database restore brings back the
// state of the backup point, which predates every withdrawal and deletion done
// since. Those actions must not be lost: a withdrawn lesson with a known error
// would reach students again, and a deleted student's data would come back. The
// restore journal records each such action, after its transaction committed, in
// an append only file that lives outside the database (and its backups), and
// POST /edu/restore/replay re-applies the entries newer than the backup point and
// reports the difference it found.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"yingce/backend/internal/model"
)

const (
	journalContentWithdrawal = "content_withdrawal" // blocking incident withdrew a publication
	journalDataWithdraw      = "data_withdraw"      // completed withdraw data request
	journalDataDelete        = "data_delete"        // completed delete data request
	// replayMargin widens the backup point: an action whose transaction started
	// shortly before the snapshot may still have committed after it.
	replayMargin  = 10 * time.Minute
	journalCtxKey = "edumath.journal"
)

// JournalEntry holds ids only: the publication or student, the incident or data
// request, and the request's kind and requester. No content, no names.
type JournalEntry struct {
	At        time.Time `json:"at"`
	Tenant    string    `json:"tenant"`
	Kind      string    `json:"kind"`
	Subject   string    `json:"subject"`
	Ref       string    `json:"ref"`
	Requester string    `json:"requester,omitempty"`
	Actor     string    `json:"actor"`
}

// RestoreJournal is an append only JSON lines file. Every append is fsynced.
type RestoreJournal struct {
	Path string
	mu   sync.Mutex
}

// JournalFromEnv returns the journal at EDU_RESTORE_JOURNAL, or nil.
func JournalFromEnv() *RestoreJournal {
	p := strings.TrimSpace(os.Getenv("EDU_RESTORE_JOURNAL"))
	if p == "" {
		return nil
	}
	return &RestoreJournal{Path: p}
}

func (j *RestoreJournal) Append(es []JournalEntry) error {
	if j == nil || len(es) == 0 {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	f, err := os.OpenFile(j.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	var b strings.Builder
	for _, e := range es {
		raw, _ := json.Marshal(e)
		b.Write(raw)
		b.WriteByte('\n')
	}
	if _, err := f.WriteString(b.String()); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Read returns the tenant's entries in journal order. A torn last line (crash
// mid write) is skipped and counted.
func (j *RestoreJournal) Read(tenant string) ([]JournalEntry, int, error) {
	f, err := os.Open(j.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	var out []JournalEntry
	bad := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var e JournalEntry
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.Kind == "" {
			bad++
			continue
		}
		if e.Tenant == tenant {
			out = append(out, e)
		}
	}
	return out, bad, sc.Err()
}

// pendingJournal collects a request's entries; Middleware flushes them only
// after the request transaction committed, so a rolled back action is never
// journaled.
type pendingJournal struct {
	j  *RestoreJournal
	es []JournalEntry
}

func (h *API) journalLater(c *gin.Context, e JournalEntry) {
	if h.Svc == nil || h.Svc.Journal == nil {
		return
	}
	if v, ok := c.Get(journalCtxKey); ok {
		if p, ok := v.(*pendingJournal); ok {
			p.es = append(p.es, e)
			return
		}
	}
	c.Set(journalCtxKey, &pendingJournal{j: h.Svc.Journal, es: []JournalEntry{e}})
}

func flushJournal(c *gin.Context) {
	v, ok := c.Get(journalCtxKey)
	if !ok {
		return
	}
	if p, ok := v.(*pendingJournal); ok {
		if err := p.j.Append(p.es); err != nil {
			// The action itself committed; replay of this entry is then only
			// covered by the in database markers. Loud so operators see it.
			log.Printf("edumath: restore journal append failed (%d entries): %v", len(p.es), err)
		}
	}
}

func (h *API) registerRestore(g *gin.RouterGroup) {
	g.POST("/restore/replay", h.restoreReplay)
}

type replayItem struct {
	At      time.Time `json:"at"`
	Kind    string    `json:"kind"`
	Subject string    `json:"subject"`
	Ref     string    `json:"ref"`
	// already_applied | reapplied | would_reapply | absent | superseded
	Outcome string `json:"outcome"`
	Detail  string `json:"detail,omitempty"`
}

// restoreReplay re-applies the journal entries newer than the backup point
// (since, RFC3339, from the backup manifest) minus replayMargin. Without
// confirm it only reports what it would change.
func (h *API) restoreReplay(c *gin.Context) {
	a, tx, ok := h.orgAdmin(c)
	if !ok {
		return
	}
	var in struct {
		Since   string `json:"since"`
		Confirm bool   `json:"confirm"`
	}
	_ = c.ShouldBindJSON(&in)
	since, err := time.Parse(time.RFC3339, strings.TrimSpace(in.Since))
	if err != nil {
		h.fail(c, appErr(400, "VALIDATION_FAILED", "since must be the backup time in RFC3339", "fix_input"))
		return
	}
	if h.Svc.Journal == nil {
		h.fail(c, appErr(409, "RESTORE_JOURNAL_MISSING", "EDU_RESTORE_JOURNAL is not configured; replay only the in database markers via /edu/data-requests/replay", "configure"))
		return
	}
	es, torn, err := h.Svc.Journal.Read(a.TenantID)
	if err != nil {
		h.fail(c, err)
		return
	}
	dry := !in.Confirm
	from := since.Add(-replayMargin)
	now := h.Svc.now()
	items := []replayItem{}
	counts := map[string]int{}
	skipped := 0
	for _, e := range es {
		if e.At.Before(from) {
			skipped++
			continue
		}
		it, err := h.Svc.replayEntry(tx, a, e, now, dry)
		if err != nil {
			h.fail(c, err)
			return
		}
		items = append(items, it)
		counts[it.Outcome]++
	}
	if !dry {
		h.Svc.audit(tx, a, "restore.replay", a.TenantID, fmt.Sprintf("since=%s entries=%d reapplied=%d already=%d absent=%d superseded=%d",
			since.UTC().Format(time.RFC3339), len(items), counts["reapplied"], counts["already_applied"], counts["absent"], counts["superseded"]))
	}
	reply(c, 200, gin.H{"dryRun": dry, "since": since.UTC(), "from": from.UTC(), "olderSkipped": skipped, "tornLines": torn,
		"entries": len(items), "outcomes": counts, "items": items}, "ok")
}

func (s *Service) replayEntry(tx *gorm.DB, a Actor, e JournalEntry, now time.Time, dry bool) (replayItem, error) {
	it := replayItem{At: e.At, Kind: e.Kind, Subject: e.Subject, Ref: e.Ref}
	switch e.Kind {
	case journalContentWithdrawal:
		return s.replayWithdrawal(tx, a, e, it, now, dry)
	case journalDataWithdraw, journalDataDelete:
		return replayDataRequest(tx, a, e, it, now, dry)
	}
	it.Outcome, it.Detail = "absent", "unknown kind"
	return it, nil
}

func (s *Service) replayWithdrawal(tx *gorm.DB, a Actor, e JournalEntry, it replayItem, now time.Time, dry bool) (replayItem, error) {
	var pub model.EduPublication
	if tx.First(&pub, "id = ? AND tenant_id = ?", e.Subject, a.TenantID).Error != nil {
		it.Outcome, it.Detail = "absent", "publication not in the restored database"
		return it, nil
	}
	var l model.EduLessonRevision
	var job model.EduMathJob
	if tx.First(&l, "id = ? AND tenant_id = ?", pub.LessonRevisionID, a.TenantID).Error != nil ||
		tx.First(&job, "id = ? AND tenant_id = ?", l.JobID, a.TenantID).Error != nil {
		it.Outcome, it.Detail = "absent", "lesson revision not in the restored database"
		return it, nil
	}
	imp := computeImpact(tx, a.TenantID, l, pub)
	var incN int64
	tx.Model(&model.EduContentIncident{}).Where("tenant_id = ? AND id = ?", a.TenantID, e.Ref).Count(&incN)
	pending := withdrawPending(tx, a.TenantID, pub, l, imp)
	if pending == 0 && incN > 0 {
		it.Outcome = "already_applied"
		return it, nil
	}
	it.Detail = fmt.Sprintf("rows_to_change=%d incident_restored=%v", pending, incN == 0)
	if dry {
		it.Outcome = "would_reapply"
		return it, nil
	}
	if err := withdrawChain(tx, a.TenantID, pub, l, imp, now); err != nil {
		return it, err
	}
	if incN == 0 {
		raw, _ := json.Marshal(imp)
		inc := model.EduContentIncident{ID: e.Ref, TenantID: a.TenantID, WorkspaceID: job.WorkspaceID, PublicationID: pub.ID, LessonRevisionID: l.ID,
			Severity: "blocking", Reason: "restore replay of the withdrawal at " + e.At.UTC().Format(time.RFC3339), State: "open", ImpactJSON: string(raw), CreatedBy: e.Actor, CreatedAt: now}
		if err := tx.Create(&inc).Error; err != nil {
			return it, err
		}
	}
	s.audit(tx, a, "restore.replay.withdrawal", e.Ref, pub.ID)
	it.Outcome = "reapplied"
	return it, nil
}

// replayDataRequest re-runs a completed withdraw or delete request unless the
// student holds an active role, consent or enrollment set after it (a legitimate
// return, for example new consent), which is reported for manual review instead.
func replayDataRequest(tx *gorm.DB, a Actor, e JournalEntry, it replayItem, now time.Time, dry bool) (replayItem, error) {
	var later int64
	for _, q := range []*gorm.DB{
		tx.Model(&model.EduRoleBinding{}).Where("tenant_id = ? AND user_id = ? AND role = ? AND status = ? AND updated_at > ?", a.TenantID, e.Subject, "student", "active", e.At),
		tx.Model(&model.EduConsentRecord{}).Where("tenant_id = ? AND student_user_id = ? AND status = ? AND updated_at > ?", a.TenantID, e.Subject, "granted", e.At),
		tx.Model(&model.EduEnrollment{}).Where("tenant_id = ? AND student_user_id = ? AND status = ? AND updated_at > ?", a.TenantID, e.Subject, "active", e.At),
	} {
		var n int64
		q.Count(&n)
		later += n
	}
	if later > 0 {
		it.Outcome, it.Detail = "superseded", fmt.Sprintf("%d role, consent or enrollment rows set active after the request; review manually", later)
		return it, nil
	}
	stop, err := stopProcessing(tx, a, e.Subject, "restore replay of data request "+e.Ref, now, dry)
	if err != nil {
		return it, err
	}
	reverted := stop["studentRoleRevoked"].(int64) + stop["consentWithdrawn"].(int64) + stop["enrollmentsWithdrawn"].(int64)
	sum := RetentionSummary{LearningRows: map[string]int{}}
	if e.Kind == journalDataDelete {
		if _, err := deleteStudentLearning(tx, a.TenantID, e.Subject, "", dry, &sum); err != nil {
			return it, err
		}
	}
	var reqN int64
	tx.Model(&model.EduDataRequest{}).Where("tenant_id = ? AND id = ?", a.TenantID, e.Ref).Count(&reqN)
	if reverted == 0 && sum.total() == 0 && reqN > 0 {
		it.Outcome = "already_applied"
		return it, nil
	}
	it.Detail = fmt.Sprintf("states=%d rows=%d request_restored=%v", reverted, sum.total(), reqN == 0)
	if dry {
		it.Outcome = "would_reapply"
		return it, nil
	}
	if reqN == 0 {
		kind := "withdraw"
		if e.Kind == journalDataDelete {
			kind = "delete"
		}
		req := e.Requester
		if req == "" {
			req = "institution"
		}
		at := e.At
		out, _ := json.Marshal(gin.H{"restoreReplay": true, "originalCompletedAt": e.At.UTC()})
		r := model.EduDataRequest{ID: e.Ref, TenantID: a.TenantID, StudentUserID: e.Subject, Kind: kind, Requester: req, Reason: "restore replay",
			Status: "completed", Outcome: string(out), ResolvedBy: e.Actor, ResolvedAt: &at, CreatedBy: a.UserID, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&r).Error; err != nil {
			return it, err
		}
	}
	it.Outcome = "reapplied"
	return it, nil
}

// journalWithdrawal and journalDataRequest build the entries the handlers queue.
func journalWithdrawal(a Actor, inc model.EduContentIncident, at time.Time) JournalEntry {
	return JournalEntry{At: at, Tenant: a.TenantID, Kind: journalContentWithdrawal, Subject: inc.PublicationID, Ref: inc.ID, Actor: a.UserID}
}

func journalDataRequest(a Actor, r model.EduDataRequest, at time.Time) JournalEntry {
	k := journalDataWithdraw
	if r.Kind == "delete" {
		k = journalDataDelete
	}
	return JournalEntry{At: at, Tenant: a.TenantID, Kind: k, Subject: r.StudentUserID, Ref: r.ID, Requester: r.Requester, Actor: a.UserID}
}
