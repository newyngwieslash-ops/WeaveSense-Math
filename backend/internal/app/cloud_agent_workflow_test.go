package app

import (
 "context"
 "encoding/json"
 "errors"
 "net/http"
 "net/http/httptest"
 "testing"
 "time"

 "gorm.io/gorm"
 "yingce/backend/internal/database"
 "yingce/backend/internal/flow"
 "yingce/backend/internal/model"
)

func TestT17WorkflowContractRejectsUnboundCalls(t *testing.T) {
 for _, raw := range []string{`{}`, `{"ops":[]}`, `{"run":{"budget_micro":100}}`, `{"ops":[],"run":{"budget_micro":100}}`, `{"expected_revision":1,"ops":[],"run":{"budget_micro":0}}`, `{"expected_revision":1,"ops":[],"run":{"budget_micro":100},"approval_id":"client"}`} {
  if _, err := decodeCloudAgentWorkflowCall(raw); err == nil { t.Fatalf("accepted incomplete/untrusted call %s",raw) }
 }
 if _,err:=decodeCloudAgentWorkflowCall(`{"expected_revision":1,"ops":[],"run":{"budget_micro":100}}`); err!=nil { t.Fatal(err) }
}

func TestT17WorkflowAuthorityDoesNotTrustApprovalID(t *testing.T) {
 s,db,_,_:=creationTestService(t)
 access:=flow.CanvasWorkflowAccess{UserID:"user",TenantID:model.PersonalTenantID("user"),WorkspaceID:model.PersonalWorkspaceID("user"),CanvasID:"canvas",RunID:"client-approved",RequestHash:"client",CanvasRevision:1,BudgetMicro:100}
 if _,err:=s.CanvasWorkflowExecutions().Authority.Authorize(context.Background(),db,access);err==nil { t.Fatal("authority accepted client identity without real approved call") }
}

func TestT17MonitorCanvasRecurringToolIsRealCompiledTool(t *testing.T) {
	s := &Service{}
	tool, err := s.MonitorCanvasRecurringTool()
	if err != nil || tool != "canvas_run_workflow" {
		t.Fatalf("recurring tool not the real compiled workflow tool: %q %v", tool, err)
	}
}

// t17WorkflowSetup provisions the real service stack for the approval chain:
// local gateway route frozen at admission, a runnable data-source→chart canvas,
// flow billing account and the managed runner. No model call happens for a pure
// data graph; the gateway only satisfies route freezing.
func t17WorkflowSetup(t *testing.T) (*Service,*gorm.DB,*httptest.Server) {
 t.Helper()
 s,db,_,_:=creationTestService(t)
 if err:=database.MigrateSchema(db);err!=nil{t.Fatal(err)}
 gw:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  w.WriteHeader(http.StatusOK)
  _=json.NewEncoder(w).Encode(map[string]any{"choices":[]any{map[string]any{"message":map[string]any{"content":"{}"}}},"usage":map[string]any{"total_tokens":1}})
 }))
 t.Cleanup(gw.Close)
 gateway:=&flow.OpenAICompatGateway{URL:gw.URL,Model:"t17-model",MicroPer1KTok:1000,HTTP:gw.Client()}
 s.ConfigureFlowModel(gateway)
 if _,err:=flow.ResolveScope(db,"user","");err!=nil{t.Fatal(err)}
 if _,err:=(flow.Billing{DB:db}).Account(db,model.PersonalTenantID("user"));err!=nil{t.Fatal(err)}
 doc:=`{"nodes":[{"id":"a","type":"data-source","metadata":{"dataSource":{"format":"json","rawText":"[{\"v\":1},{\"v\":2}]"},"content":"old"}},{"id":"b","type":"chart","metadata":{"content":"stale","computed":[9]}}],"connections":[{"id":"e","fromNodeId":"a","toNodeId":"b"}]}`
 if err:=db.Create(&model.CanvasProject{ID:"cv17",UserID:"user",Revision:1,PayloadJSON:doc}).Error;err!=nil{t.Fatal(err)}
 runner:=flow.CanvasWorkflowManagedRunner{Tools:flow.Tools{DB:db,Model:gateway},WorkerID:"t17-runner",Timeout:15*time.Second}
 s.ConfigureCanvasWorkflowRunner(runner.Run)
 return s,db,gw
}

func t17RequestApproval(t *testing.T,s *Service,raw string) (string,string) {
 t.Helper()
 out,err:=s.RequestCanvasWorkflowApproval(context.Background(),"user","cv17",json.RawMessage(raw))
 if err!=nil{t.Fatal(err)}
 agent,ok1:=out["agent_run_id"].(string)
 approval,ok2:=out["approval_id"].(string)
 if !ok1||!ok2||agent==""||approval==""{t.Fatalf("approval response: %+v",out)}
 return agent,approval
}

func TestT17RealServiceDecisionAdmitManagedRunAndResume(t *testing.T) {
 s,db,_:=t17WorkflowSetup(t)
 var executed,reserved int64
 db.Model(&model.CanvasWorkflowExecution{}).Count(&executed)
 db.Model(&model.FlowBudgetReservation{}).Count(&reserved)
 if executed!=0||reserved!=0{t.Fatalf("setup leaked rows: %d/%d",executed,reserved)}
 agent,approval:=t17RequestApproval(t,s,`{"expected_revision":1,"ops":[],"run":{"budget_micro":5000}}`)
 db.Model(&model.CanvasWorkflowExecution{}).Count(&executed)
 db.Model(&model.FlowBudgetReservation{}).Count(&reserved)
 if executed!=0||reserved!=0{t.Fatalf("request is not control-plane only: %d/%d",executed,reserved)}
 // The real decision path drives consume -> Admit -> managed run.
 if err:=s.DecideCloudAgentApproval("user",agent,approval,"approve","");err!=nil{t.Fatal(err)}
 var record model.CanvasWorkflowExecution
 deadline:=time.Now().Add(20*time.Second)
 for time.Now().Before(deadline) {
  db.Where("user_id = ? AND canvas_id = ?","user","cv17").First(&record)
  // saving_result/saving_report are transient commit stages, not terminal.
  switch record.Status {
  case "succeeded","failed","cancelled","waiting_external":
   deadline=time.Now()
  default:
   time.Sleep(50*time.Millisecond)
  }
 }
 if record.Status!="succeeded"{t.Fatalf("managed run terminal state: %s %+v",record.Status,record)}
 view,err:=s.GetCanvasWorkflow(context.Background(),"user","cv17",record.ID)
 if err!=nil||view["status"]!="succeeded"||view["revision"]!=int64(1){t.Fatalf("get view: %+v %v",view,err)}
 var project model.CanvasProject
 if err=db.First(&project,"id = ?","cv17").Error;err!=nil{t.Fatal(err)}
 if project.Revision!=2{t.Fatalf("canvas not committed: %d",project.Revision)}
 var payload map[string]any
 if err=json.Unmarshal([]byte(project.PayloadJSON),&payload);err!=nil{t.Fatal(err)}
 if payload["workflowRunId"]!=record.ID{t.Fatalf("commit marker missing: %+v",payload["workflowRunId"])}
 chart:=payload["nodes"].([]any)[1].(map[string]any)["metadata"].(map[string]any)
 if len(chart["computed"].([]any))!=2||chart["computedBranches"]==nil||chart["derived"]==nil{t.Fatalf("authoritative chart output missing: %+v",chart)}
 // Replay through the explicit run entry returns the same durable run.
 again,err:=s.RunApprovedCanvasWorkflow(context.Background(),"user","cv17",agent,approval)
 if err!=nil||again.ID!=record.ID{t.Fatalf("replay: %+v %v",again,err)}
 var runs int64
 db.Model(&model.CanvasWorkflowExecution{}).Count(&runs)
 if runs!=1{t.Fatalf("duplicate executions: %d",runs)}
 var reservations []model.FlowBudgetReservation
 if err=db.Find(&reservations).Error;err!=nil{t.Fatal(err)}
 if len(reservations)!=1||reservations[0].Status!="settled"||reservations[0].ReservedMicro!=5000{t.Fatalf("billing not settled once: %+v",reservations)}
 // Same-session restore: the parked wait resolves the manual-approval root.
 var wait model.FlowExternalWait
 if err=db.First(&wait,"operation_id = ?",record.ID).Error;err!=nil{t.Fatal(err)}
 if _,err=s.resumeCanvasWorkflowWait(wait);err!=nil{t.Fatal(err)}
 root,err:=s.repo.CloudAgent("user",agent)
 if err!=nil||root.Status!="completed"{t.Fatalf("manual approval root not completed: %s %v",root.Status,err)}
}

func TestT17RejectAndStaleApprovalHaveNoSideEffects(t *testing.T) {
 t.Run("reject",func(t *testing.T){
  s,db,_:=t17WorkflowSetup(t)
  agent,approval:=t17RequestApproval(t,s,`{"expected_revision":1,"ops":[],"run":{"budget_micro":5000}}`)
  if err:=s.DecideCloudAgentApproval("user",agent,approval,"reject","not now");err!=nil{t.Fatal(err)}
  if _,err:=s.RunApprovedCanvasWorkflow(context.Background(),"user","cv17",agent,approval);!errors.Is(err,flow.ErrApprovalRequired){t.Fatalf("rejected approval executed: %v",err)}
  var executed,reserved int64
  db.Model(&model.CanvasWorkflowExecution{}).Count(&executed)
  db.Model(&model.FlowBudgetReservation{}).Count(&reserved)
  if executed!=0||reserved!=0{t.Fatalf("reject side effects: %d/%d",executed,reserved)}
  root,err:=s.repo.CloudAgent("user",agent)
  if err!=nil||root.Status!="rejected"{t.Fatalf("reject terminal state: %s %v",root.Status,err)}
 })
 t.Run("stale_revision",func(t *testing.T){
  s,db,_:=t17WorkflowSetup(t)
  agent,approval:=t17RequestApproval(t,s,`{"expected_revision":1,"ops":[],"run":{"budget_micro":5000}}`)
  if err:=db.Model(&model.CanvasProject{}).Where("id = ?","cv17").Updates(map[string]any{"revision":2,"payload_json":`{"nodes":[{"id":"a","type":"data-source","metadata":{"dataSource":{"format":"json","rawText":"[]"},"content":""}}],"connections":[]}`}).Error;err!=nil{t.Fatal(err)}
  if err:=s.DecideCloudAgentApproval("user",agent,approval,"approve","");!errors.Is(err,flow.ErrCanvasWorkflowConflict){t.Fatalf("stale approval accepted: %v",err)}
  if _,err:=s.RunApprovedCanvasWorkflow(context.Background(),"user","cv17",agent,approval);!errors.Is(err,flow.ErrCanvasWorkflowConflict){t.Fatalf("stale replay accepted: %v",err)}
  var executed,reserved int64
  db.Model(&model.CanvasWorkflowExecution{}).Count(&executed)
  db.Model(&model.FlowBudgetReservation{}).Count(&reserved)
  if executed!=0||reserved!=0{t.Fatalf("stale side effects: %d/%d",executed,reserved)}
  var project model.CanvasProject
  db.First(&project,"id = ?","cv17")
  if project.Revision!=2{t.Fatalf("stale run mutated canvas: %d",project.Revision)}
 })
}
