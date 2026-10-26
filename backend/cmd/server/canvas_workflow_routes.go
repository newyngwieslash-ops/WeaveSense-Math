package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
	"yingce/backend/internal/service"
)

func registerCanvasWorkflowAppRoutes(g *gin.RouterGroup, svc *service.Service, user func(*gin.Context) (*model.User, error)) {
	respond := func(c *gin.Context, status int, data any, err error) {
		if err != nil {
			code := flow.CanvasWorkflowHTTPStatus(err)
			var appErr *service.AppError
			if errors.As(err, &appErr) {
				code = appErr.Status
			}
			c.JSON(code, gin.H{"code": code, "msg": "画布工作流请求失败", "reason": canvasWorkflowReason(err), "data": nil})
			return
		}
		c.JSON(status, gin.H{"code": 0, "data": data, "msg": "ok"})
	}
	decode := func(c *gin.Context, out any) error {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128<<10)
		d := json.NewDecoder(c.Request.Body)
		d.DisallowUnknownFields()
		if err := d.Decode(out); err != nil {
			return flow.ErrInvalid
		}
		if err := d.Decode(new(any)); err != io.EOF {
			return flow.ErrInvalid
		}
		return nil
	}
	g.POST("/canvas-workflows/:canvasId/approvals", func(c *gin.Context) {
		u, err := user(c)
		if err != nil {
			respond(c, 0, nil, err)
			return
		}
		var raw json.RawMessage
		if err = decode(c, &raw); err != nil {
			respond(c, 0, nil, err)
			return
		}
		out, err := svc.RequestCanvasWorkflowApproval(c.Request.Context(), u.ID, c.Param("canvasId"), raw)
		respond(c, 201, out, err)
	})
	g.POST("/canvas-workflows/:canvasId/run", func(c *gin.Context) {
		u, err := user(c)
		if err != nil {
			respond(c, 0, nil, err)
			return
		}
		var body struct {
			AgentRunID string `json:"agent_run_id"`
			ApprovalID string `json:"approval_id"`
		}
		if err = decode(c, &body); err != nil {
			respond(c, 0, nil, err)
			return
		}
		out, err := svc.RunApprovedCanvasWorkflow(c.Request.Context(), u.ID, c.Param("canvasId"), body.AgentRunID, body.ApprovalID)
		respond(c, 202, gin.H{"run_id": out.ID, "status": out.Status, "revision": out.CanvasRevision}, err)
	})
	g.GET("/canvas-workflows/:canvasId/runs/:runId", func(c *gin.Context) {
		u, err := user(c)
		if err != nil {
			respond(c, 0, nil, err)
			return
		}
		out, err := svc.GetCanvasWorkflow(c.Request.Context(), u.ID, c.Param("canvasId"), c.Param("runId"))
		respond(c, 200, out, err)
	})
	g.POST("/canvas-workflows/:canvasId/runs/:runId/retry-save", func(c *gin.Context) {
		u, err := user(c)
		if err != nil {
			respond(c, 0, nil, err)
			return
		}
		err = svc.RetryCanvasWorkflowSave(c.Request.Context(), u.ID, c.Param("canvasId"), c.Param("runId"))
		respond(c, 202, gin.H{"run_id": c.Param("runId"), "accepted": true}, err)
	})
	g.POST("/canvas-workflows/:canvasId/runs/:runId/cancel", func(c *gin.Context) {
		u, err := user(c)
		if err != nil {
			respond(c, 0, nil, err)
			return
		}
		if _, err = svc.GetCanvasWorkflow(c.Request.Context(), u.ID, c.Param("canvasId"), c.Param("runId")); err == nil {
			err = flow.CancelCanvasWorkflow(c.Request.Context(), svc.CanvasWorkflowExecutions(), u.ID, c.Param("runId"))
		}
		respond(c, 202, gin.H{"run_id": c.Param("runId"), "accepted": true}, err)
	})
}
func canvasWorkflowReason(err error) string {
	switch {
	case errors.Is(err, flow.ErrApprovalRequired):
		return "canvas_approval_required"
	case errors.Is(err, flow.ErrCanvasWorkflowConflict):
		return "canvas_revision_conflict"
	case errors.Is(err, flow.ErrForbidden):
		return "canvas_permission_denied"
	case errors.Is(err, flow.ErrInvalid):
		return "invalid_canvas_workflow"
	default:
		return "canvas_workflow_failed"
	}
}
