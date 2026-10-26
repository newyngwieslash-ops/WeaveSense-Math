package edumath

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

// Demo class (DEV-067, 05 question 17 decided 2026-10-26): every workspace may hold one demo class
// with fictional students and fictional learning records so a new teacher can see the dashboard
// before a real class exists. It lives only in edu_demo_classes and edu_demo_records (FORCE RLS):
// no edu_classes row, no enrollment, attempt, help request, usage or ledger row is ever written, so
// it cannot reach real classes, students, metrics, billing, dashboards or exports. Every response
// carries demo:true and the label. Teachers and org admins of the workspace create, reset and remove
// it; students have no route to it.

const (
	demoLabel       = "演示数据：虚构班级与虚构学习记录，不是真实学生，不计入任何统计、计费与导出"
	demoSeedVersion = 1
	demoClassName   = "示例班级（演示数据）"
)

type demoSeed struct {
	Student, Kind, Item, Result, Message string
	Hint                                 int
}

// Fictional names only; none is a real person.
var demoStudents = []string{"示例学生甲", "示例学生乙", "示例学生丙", "示例学生丁", "示例学生戊", "示例学生己"}

var demoSeeds = []demoSeed{
	{Student: "示例学生甲", Kind: "attempt", Item: "main", Result: "correct"},
	{Student: "示例学生乙", Kind: "hint", Item: "main", Hint: 1},
	{Student: "示例学生乙", Kind: "attempt", Item: "main", Result: "correct"},
	{Student: "示例学生丙", Kind: "attempt", Item: "main", Result: "incorrect", Message: "把侧棱与底面所成角当成了侧面与底面所成角"},
	{Student: "示例学生丙", Kind: "hint", Item: "main", Hint: 1},
	{Student: "示例学生丙", Kind: "hint", Item: "main", Hint: 2},
	{Student: "示例学生丁", Kind: "attempt", Item: "main", Result: "incorrect", Message: "射影位置找错，取了底边中点"},
	{Student: "示例学生丁", Kind: "help", Item: "beat:s2", Message: "第二步为什么顶点的射影落在底面中心？"},
	{Student: "示例学生戊", Kind: "attempt", Item: "main", Result: "correct_approx"},
	{Student: "示例学生己", Kind: "help", Item: "beat:s3", Message: "余弦值怎么化成角度？"},
}

type demoDashboard struct {
	Students     int            `json:"students"`
	Attempted    int            `json:"attempted"`
	Correct      int            `json:"correct"`
	HintUsers    int            `json:"hintUsers"`
	HintsByLevel map[int]int    `json:"hintsByLevel"`
	Results      map[string]int `json:"results"`
	CommonErrors []string       `json:"commonErrors"`
	OpenHelp     int            `json:"openHelp"`
}

func demoAggregate(recs []model.EduDemoRecord) demoDashboard {
	d := demoDashboard{Students: len(demoStudents), HintsByLevel: map[int]int{}, Results: map[string]int{}, CommonErrors: []string{}}
	attempted, hinted := map[string]bool{}, map[string]bool{}
	for _, r := range recs {
		switch r.Kind {
		case "attempt":
			if !attempted[r.StudentName] {
				attempted[r.StudentName] = true
			}
			d.Results[r.Result]++
			if r.Result == "correct" || r.Result == "correct_approx" {
				d.Correct++
			}
			if r.Message != "" {
				d.CommonErrors = append(d.CommonErrors, r.Message)
			}
		case "hint":
			hinted[r.StudentName] = true
			d.HintsByLevel[r.HintLevel]++
		case "help":
			d.OpenHelp++
		}
	}
	d.Attempted, d.HintUsers = len(attempted), len(hinted)
	return d
}

func (h *API) registerDemo(g *gin.RouterGroup) {
	g.GET("/demo-class", h.getDemoClass)
	g.POST("/demo-class/reset", h.resetDemoClass)
	g.POST("/demo-class/remove", h.removeDemoClass)
}

func demoStaff(h *API, c *gin.Context) (Actor, *gorm.DB, bool) {
	a, tx, ok := h.staff(c)
	if !ok {
		return a, tx, false
	}
	if !a.Has("teacher", "org_admin") {
		h.fail(c, appErr(http.StatusForbidden, "FORBIDDEN", "只有教师或机构管理员可以管理示例班级", "联系机构管理员"))
		return a, tx, false
	}
	return a, tx, true
}

// demoSummary is the home view of the demo class; nil when the workspace has none.
func demoSummary(tx *gorm.DB, a Actor) gin.H {
	var k model.EduDemoClass
	if tx.Where("tenant_id = ? AND workspace_id = ?", a.TenantID, a.WorkspaceID).Limit(1).Find(&k).RowsAffected == 0 {
		return nil
	}
	return gin.H{"demo": true, "label": demoLabel, "id": k.ID, "name": k.Name, "students": len(demoStudents)}
}

func (h *API) demoView(tx *gorm.DB, a Actor) (gin.H, bool) {
	var k model.EduDemoClass
	if tx.Where("tenant_id = ? AND workspace_id = ?", a.TenantID, a.WorkspaceID).Limit(1).Find(&k).RowsAffected == 0 {
		return gin.H{"demo": true, "label": demoLabel, "class": nil}, false
	}
	var recs []model.EduDemoRecord
	tx.Where("tenant_id = ? AND demo_class_id = ?", a.TenantID, k.ID).Order("created_at, id").Find(&recs)
	return gin.H{"demo": true, "label": demoLabel, "class": gin.H{"demo": true, "id": k.ID, "name": k.Name, "seedVersion": k.SeedVersion, "createdAt": k.CreatedAt},
		"students": demoStudents, "records": recs, "dashboard": demoAggregate(recs)}, true
}

func (h *API) getDemoClass(c *gin.Context) {
	a, tx, ok := demoStaff(h, c)
	if !ok {
		return
	}
	v, _ := h.demoView(tx, a)
	reply(c, http.StatusOK, v, "ok")
}

func deleteDemo(tx *gorm.DB, a Actor) error {
	var ids []string
	tx.Model(&model.EduDemoClass{}).Where("tenant_id = ? AND workspace_id = ?", a.TenantID, a.WorkspaceID).Pluck("id", &ids)
	if len(ids) == 0 {
		return nil
	}
	if err := tx.Where("tenant_id = ? AND demo_class_id IN ?", a.TenantID, ids).Delete(&model.EduDemoRecord{}).Error; err != nil {
		return err
	}
	return tx.Where("tenant_id = ? AND id IN ?", a.TenantID, ids).Delete(&model.EduDemoClass{}).Error
}

// resetDemoClass creates the demo class or restores it to the fixed seed (idempotent).
func (h *API) resetDemoClass(c *gin.Context) {
	a, tx, ok := demoStaff(h, c)
	if !ok {
		return
	}
	if err := deleteDemo(tx, a); err != nil {
		h.fail(c, err)
		return
	}
	now := h.Svc.now()
	k := model.EduDemoClass{ID: flow.NewID("edc_"), TenantID: a.TenantID, WorkspaceID: a.WorkspaceID, Name: demoClassName, SeedVersion: demoSeedVersion, CreatedBy: a.UserID, CreatedAt: now}
	if err := tx.Create(&k).Error; err != nil {
		h.fail(c, err)
		return
	}
	for i, s := range demoSeeds {
		r := model.EduDemoRecord{ID: flow.NewID("edr_"), TenantID: a.TenantID, DemoClassID: k.ID, StudentName: s.Student, Kind: s.Kind, Item: s.Item,
			Result: s.Result, HintLevel: s.Hint, Message: s.Message, CreatedAt: now.Add(time.Duration(i) * time.Minute)}
		if err := tx.Create(&r).Error; err != nil {
			h.fail(c, err)
			return
		}
	}
	h.Svc.audit(tx, a, "demo_class.reset", k.ID, "demo")
	v, _ := h.demoView(tx, a)
	reply(c, http.StatusOK, v, "ok")
}

func (h *API) removeDemoClass(c *gin.Context) {
	a, tx, ok := demoStaff(h, c)
	if !ok {
		return
	}
	if err := deleteDemo(tx, a); err != nil {
		h.fail(c, err)
		return
	}
	h.Svc.audit(tx, a, "demo_class.remove", a.WorkspaceID, "demo")
	reply(c, http.StatusOK, gin.H{"demo": true, "label": demoLabel, "class": nil}, "ok")
}
