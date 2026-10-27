package edumath

// Teacher active time and teaching metrics (DEV-117).
//
// Active time is measured, not guessed: the teacher pages send a beat about once a minute only while the
// page is visible and the teacher interacted within the last minute. The server adds the real time since
// the teacher's previous beat when that gap is within the idle threshold, and nothing otherwise, so idle
// periods, closed laptops and parallel tabs never inflate the total. A teacher may correct a day and phase
// with a time diary entry; reports show measured, diary and corrected values side by side.
//
// Teaching metrics for an assignment keep independent, assisted, missing and demo results apart and always
// show the denominator, the sample size and the definition version.

import (
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

const (
	// teacherIdle is the idle threshold: a gap longer than this between beats is not counted.
	teacherIdle            = 5 * time.Minute
	teacherTimeDefinition  = "teacher_time/1"
	teachingMetricsVersion = "teaching_metrics/1"
	maxDiaryMinutes        = 16 * 60
	maxTimeRangeDays       = 92
)

var (
	teacherPhases = []string{"prep", "review", "edit", "publish", "after_class"}
	dayRe         = regexp.MustCompile(`^20[0-9]{2}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])$`)
	// reportZone is the zone that assigns active time to calendar days (China Standard Time, no DST).
	reportZone = time.FixedZone("UTC+8", 8*3600)
)

func validPhase(p string) bool {
	for _, x := range teacherPhases {
		if x == p {
			return true
		}
	}
	return false
}

func (h *API) registerTeacherTime(g *gin.RouterGroup) {
	g.POST("/teacher-time/beat", h.teacherBeat)
	g.PUT("/teacher-time/diary", h.teacherDiary)
	g.GET("/teacher-time", h.teacherTime)
	g.GET("/assignments/:id/teaching-metrics", h.teachingMetrics)
}

// recordBeat adds the active gap since the teacher's previous beat to the segment for phase and object.
func recordBeat(tx *gorm.DB, a Actor, phase, object string, now time.Time) (model.EduTeacherTimeSegment, int, error) {
	var last model.EduTeacherTimeSegment
	gap := 0
	if tx.Where("tenant_id = ? AND teacher_user_id = ? AND last_beat_at > ?", a.TenantID, a.UserID, now.Add(-teacherIdle)).
		Order("last_beat_at desc").Take(&last).Error == nil {
		if d := now.Sub(last.LastBeatAt); d > 0 && d <= teacherIdle {
			gap = int(d / time.Second)
		}
	}
	day := now.In(reportZone).Format("2006-01-02")
	var seg model.EduTeacherTimeSegment
	err := tx.Where("tenant_id = ? AND teacher_user_id = ? AND day = ? AND phase = ? AND object_id = ? AND last_beat_at > ?",
		a.TenantID, a.UserID, day, phase, object, now.Add(-teacherIdle)).Order("last_beat_at desc").Take(&seg).Error
	if err != nil {
		seg = model.EduTeacherTimeSegment{ID: flow.NewID("ets_"), TenantID: a.TenantID, TeacherUserID: a.UserID, Day: day, Phase: phase, ObjectID: object,
			StartedAt: now, LastBeatAt: now, ActiveSeconds: gap, Beats: 1}
		return seg, gap, tx.Create(&seg).Error
	}
	seg.ActiveSeconds += gap
	seg.Beats++
	if now.After(seg.LastBeatAt) {
		seg.LastBeatAt = now
	}
	return seg, gap, tx.Model(&model.EduTeacherTimeSegment{}).Where("id = ?", seg.ID).
		Updates(map[string]any{"active_seconds": seg.ActiveSeconds, "beats": seg.Beats, "last_beat_at": seg.LastBeatAt}).Error
}

func (h *API) teacherBeat(c *gin.Context) {
	a, tx, ok := h.staff(c)
	if !ok {
		return
	}
	var in struct {
		Phase    string `json:"phase"`
		ObjectID string `json:"objectId"`
	}
	if c.ShouldBindJSON(&in) != nil || !validPhase(in.Phase) || (in.ObjectID != "" && !metricObjRe.MatchString(in.ObjectID)) {
		h.fail(c, appErr(400, "VALIDATION_FAILED", "phase must be prep, review, edit, publish or after_class; objectId must be an object id", "fix_input"))
		return
	}
	seg, gap, err := recordBeat(tx, a, in.Phase, in.ObjectID, h.Svc.now())
	if err != nil {
		h.fail(c, err)
		return
	}
	reply(c, 200, gin.H{"segmentId": seg.ID, "day": seg.Day, "phase": seg.Phase, "addedSeconds": gap, "activeSeconds": seg.ActiveSeconds}, "ok")
}

func (h *API) teacherDiary(c *gin.Context) {
	a, tx, ok := h.staff(c)
	if !ok {
		return
	}
	var in struct {
		Day     string `json:"day"`
		Phase   string `json:"phase"`
		Minutes *int   `json:"minutes"`
	}
	if c.ShouldBindJSON(&in) != nil || !dayRe.MatchString(in.Day) || !validPhase(in.Phase) || in.Minutes == nil || *in.Minutes < 0 || *in.Minutes > maxDiaryMinutes {
		h.fail(c, appErr(400, "VALIDATION_FAILED", fmt.Sprintf("day (YYYY-MM-DD), phase and minutes (0 to %d) required", maxDiaryMinutes), "fix_input"))
		return
	}
	d, err := time.ParseInLocation("2006-01-02", in.Day, reportZone)
	now := h.Svc.now()
	if err != nil || d.After(now) {
		h.fail(c, appErr(400, "VALIDATION_FAILED", "day must be a real date, not in the future", "fix_input"))
		return
	}
	var e model.EduTeacherTimeDiary
	if tx.First(&e, "tenant_id = ? AND teacher_user_id = ? AND day = ? AND phase = ?", a.TenantID, a.UserID, in.Day, in.Phase).Error == nil {
		err = tx.Model(&model.EduTeacherTimeDiary{}).Where("id = ?", e.ID).Updates(map[string]any{"minutes": *in.Minutes, "updated_at": now}).Error
		e.Minutes, e.UpdatedAt = *in.Minutes, now
	} else {
		e = model.EduTeacherTimeDiary{ID: flow.NewID("etd_"), TenantID: a.TenantID, TeacherUserID: a.UserID, Day: in.Day, Phase: in.Phase, Minutes: *in.Minutes, CreatedAt: now, UpdatedAt: now}
		err = tx.Create(&e).Error
	}
	if err != nil {
		h.fail(c, err)
		return
	}
	reply(c, 200, e, "ok")
}

// teacherTime reports active time per phase for the caller, or (scope=org, org admins only) the sum over
// all teachers of the tenant without a per-teacher breakdown.
func (h *API) teacherTime(c *gin.Context) {
	a, tx, ok := h.staff(c)
	if !ok {
		return
	}
	now := h.Svc.now().In(reportZone)
	to, from := now.Format("2006-01-02"), now.AddDate(0, 0, -6).Format("2006-01-02")
	if v := c.Query("from"); v != "" {
		from = v
	}
	if v := c.Query("to"); v != "" {
		to = v
	}
	f, e1 := time.Parse("2006-01-02", from)
	t, e2 := time.Parse("2006-01-02", to)
	if !dayRe.MatchString(from) || !dayRe.MatchString(to) || e1 != nil || e2 != nil || t.Before(f) || t.Sub(f) > maxTimeRangeDays*24*time.Hour {
		h.fail(c, appErr(400, "VALIDATION_FAILED", fmt.Sprintf("from and to must be days (YYYY-MM-DD), from not after to, at most %d days apart", maxTimeRangeDays), "fix_input"))
		return
	}
	org := c.Query("scope") == "org"
	if org && !a.Has("org_admin") && !canManageRoles(tx, a.TenantID, a.UserID) {
		h.fail(c, ErrForbidden)
		return
	}
	scope := func(q *gorm.DB) *gorm.DB {
		q = q.Where("tenant_id = ? AND day >= ? AND day <= ?", a.TenantID, from, to)
		if !org {
			q = q.Where("teacher_user_id = ?", a.UserID)
		}
		return q
	}
	var segs []model.EduTeacherTimeSegment
	var diaries []model.EduTeacherTimeDiary
	if err := scope(tx.Model(&model.EduTeacherTimeSegment{})).Find(&segs).Error; err != nil {
		h.fail(c, err)
		return
	}
	if err := scope(tx.Model(&model.EduTeacherTimeDiary{})).Find(&diaries).Error; err != nil {
		h.fail(c, err)
		return
	}
	type key struct{ teacher, day, phase string }
	measured := map[key]int{}
	diary := map[key]int{}
	teachers := map[string]bool{}
	days := map[string]bool{}
	for _, s := range segs {
		measured[key{s.TeacherUserID, s.Day, s.Phase}] += s.ActiveSeconds
		teachers[s.TeacherUserID], days[s.Day] = true, true
	}
	for _, d := range diaries {
		diary[key{d.TeacherUserID, d.Day, d.Phase}] = d.Minutes
		teachers[d.TeacherUserID], days[d.Day] = true, true
	}
	type phaseSum struct {
		MeasuredSeconds  int `json:"measuredSeconds"`
		DiaryMinutes     int `json:"diaryMinutes"`
		DiaryEntries     int `json:"diaryEntries"`
		CorrectedSeconds int `json:"correctedSeconds"`
	}
	phases := map[string]*phaseSum{}
	for _, p := range teacherPhases {
		phases[p] = &phaseSum{}
	}
	all := map[key]bool{}
	for k := range measured {
		all[k] = true
	}
	for k := range diary {
		all[k] = true
	}
	var total phaseSum
	for k := range all {
		p := phases[k.phase]
		if p == nil {
			continue
		}
		m := measured[k]
		p.MeasuredSeconds += m
		corrected := m
		if d, ok := diary[k]; ok {
			p.DiaryMinutes += d
			p.DiaryEntries++
			corrected = d * 60
		}
		p.CorrectedSeconds += corrected
		total.MeasuredSeconds += m
		total.CorrectedSeconds += corrected
	}
	dayList := make([]string, 0, len(days))
	for d := range days {
		dayList = append(dayList, d)
	}
	sort.Strings(dayList)
	out := gin.H{"from": from, "to": to, "scope": "self", "phases": phases, "order": teacherPhases, "total": total, "daysWithActivity": dayList,
		"idleThresholdSeconds": int(teacherIdle / time.Second), "zone": "UTC+8", "definition": teacherTimeDefinition}
	if org {
		out["scope"], out["teachers"] = "org", len(teachers)
		out["smallSample"] = len(teachers) < metricSmallSample
	}
	reply(c, 200, out, "ok")
}

// teachingMetrics reports one assignment's results per active student: independent correct, correct
// only with hints (assisted), not yet correct, awaiting judgement and missing, with the denominator,
// first try correctness, independent self tests, and demo data counted separately.
func (h *API) teachingMetrics(c *gin.Context) {
	a, tx, as, ok := h.staffAssignment(c)
	if !ok {
		return
	}
	var roster []string
	tx.Model(&model.EduEnrollment{}).Where("tenant_id = ? AND class_id = ? AND status = ?", a.TenantID, as.ClassID, "active").Pluck("student_user_id", &roster)
	active := map[string]bool{}
	for _, s := range roster {
		active[s] = true
	}
	var ats []model.EduAttempt
	tx.Where("tenant_id = ? AND assignment_id = ? AND item = ? AND review_item_id = ?", a.TenantID, as.ID, "main", "").Order("created_at").Find(&ats)
	per := map[string][]model.EduAttempt{}
	excluded := map[string]bool{}
	for _, at := range ats {
		if !active[at.StudentUserID] {
			excluded[at.StudentUserID] = true
			continue
		}
		per[at.StudentUserID] = append(per[at.StudentUserID], at)
	}
	outcome := map[string]int{"independent": 0, "assisted": 0, "not_correct": 0, "pending": 0, "missing": 0}
	firstTry, firstTryN := 0, 0
	for _, s := range roster {
		var indep, assisted, pending, valid bool
		for _, at := range per[s] {
			if !at.Valid {
				continue
			}
			valid = true
			switch statusLabel(effective(at)) {
			case "correct":
				if at.Independent {
					indep = true
				} else {
					assisted = true
				}
			case "pending_judgement":
				pending = true
			}
			if at.FirstValid {
				firstTryN++
				if at.Independent && statusLabel(effective(at)) == "correct" {
					firstTry++
				}
			}
		}
		switch {
		case !valid:
			outcome["missing"]++
		case indep:
			outcome["independent"]++
		case assisted:
			outcome["assisted"]++
		case pending:
			outcome["pending"]++
		default:
			outcome["not_correct"]++
		}
	}
	var selfTests int64
	if len(roster) > 0 {
		tx.Model(&model.EduSelfTest{}).Where("tenant_id = ? AND assignment_id = ? AND state = ? AND student_user_id IN ?", a.TenantID, as.ID, "submitted", roster).Count(&selfTests)
	}
	var demoClasses, demoRecords int64
	tx.Model(&model.EduDemoClass{}).Where("tenant_id = ?", a.TenantID).Count(&demoClasses)
	tx.Model(&model.EduDemoRecord{}).Where("tenant_id = ?", a.TenantID).Count(&demoRecords)
	var pub model.EduPublication
	tx.Select("id", "lesson_revision_id").Take(&pub, "id = ? AND tenant_id = ?", as.PublicationID, a.TenantID)
	attempted := len(roster) - outcome["missing"]
	reply(c, 200, gin.H{
		"assignmentId": as.ID, "denominator": len(roster), "attempted": attempted, "outcomes": outcome,
		"firstTry":              gin.H{"correct": firstTry, "of": firstTryN},
		"independentSelfTests":  selfTests,
		"excludedOutsideRoster": len(excluded),
		"smallSample":           attempted < metricSmallSample,
		"demo":                  gin.H{"classes": demoClasses, "records": demoRecords, "included": false},
		"version":               gin.H{"definition": teachingMetricsVersion, "publicationId": pub.ID, "lessonRevisionId": pub.LessonRevisionID},
		"generatedAt":           h.Svc.now(),
	}, "ok")
}
