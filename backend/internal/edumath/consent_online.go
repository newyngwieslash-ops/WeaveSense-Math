package edumath

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

// Optional guardian online confirmation (DEV-017, E98). A school administrator
// issues a one-time token; the guardian submits it on a public route without a
// platform account. Offline evidenceRef recording stays unchanged. This is not
// a guardian portal (EXT-09): the school still verifies identity before it
// sends the link.

const (
	guardianNoticeVersion = "edu.guardian-notice/1"
	maxConsentInviteTTL   = 14 * 24 * time.Hour
	consentTokenBytes     = 24 // 48 hex characters, same strength as class invites
)

func guardianNotice() string {
	return "本确认仅针对该学生在本机构教育平台上的学习处理（作答、讲解播放、学情记录）。请在学校已向你说明处理目的、范围与撤回方式后再确认。撤回后平台停止新的个人化处理，已依法保留的记录按保留政策处理。此渠道不是完整的监护人门户，发放链接前的身份核验由学校完成。"
}

func (h *API) issueConsentInvite(c *gin.Context) {
	a, tx, ok := h.staff(c)
	if !ok {
		return
	}
	if !canManageRoles(tx, a.TenantID, a.UserID) {
		h.fail(c, ErrForbidden)
		return
	}
	var in struct {
		StudentUserID string `json:"studentUserId"`
		Purpose       string `json:"purpose"`
		TTLHours      int    `json:"ttlHours"`
	}
	if c.ShouldBindJSON(&in) != nil || in.StudentUserID == "" || (in.Purpose != "grant" && in.Purpose != "withdraw") {
		reply(c, 400, nil, "studentUserId and purpose (grant|withdraw) required")
		return
	}
	if consentMode(tx, a.TenantID) == "not_applicable" {
		h.fail(c, appErr(http.StatusConflict, "CONSENT_NOT_APPLICABLE", "this institution marked guardian consent not applicable", "check_policy"))
		return
	}
	rs, _ := roles(tx, a.TenantID, in.StudentUserID)
	if !rs["student"] {
		h.fail(c, ErrNotFound)
		return
	}
	if in.Purpose == "withdraw" && !consentOK(tx, a.TenantID, in.StudentUserID) {
		h.fail(c, appErr(http.StatusConflict, "CONSENT_NOT_GRANTED", "there is no granted consent to withdraw", "issue_grant_first"))
		return
	}
	ttl := time.Duration(in.TTLHours) * time.Hour
	if ttl <= 0 || ttl > maxConsentInviteTTL {
		ttl = 7 * 24 * time.Hour
	}
	buf := make([]byte, consentTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		h.fail(c, err)
		return
	}
	secret := hex.EncodeToString(buf)
	now := h.Svc.now()
	inv := model.EduConsentInvite{
		ID: flow.NewID("ect_"), TenantID: a.TenantID, StudentUserID: in.StudentUserID,
		Purpose: in.Purpose, TokenHash: secretHash(secret), NoticeVersion: guardianNoticeVersion,
		Status: "active", ExpiresAt: now.Add(ttl), IssuedBy: a.UserID, CreatedAt: now, UpdatedAt: now,
	}
	if err := tx.Create(&inv).Error; err != nil {
		h.fail(c, err)
		return
	}
	h.Svc.audit(tx, a, "consent.online_invite", in.StudentUserID, inv.ID+" "+in.Purpose)
	reply(c, 201, gin.H{
		"invite":        inv,
		"token":         inv.ID + "." + secret,
		"noticeVersion": guardianNoticeVersion,
		"notice":        guardianNotice(),
	}, "ok")
}

func (h *API) revokeConsentInvite(c *gin.Context) {
	a, tx, ok := h.staff(c)
	if !ok {
		return
	}
	if !canManageRoles(tx, a.TenantID, a.UserID) {
		h.fail(c, ErrForbidden)
		return
	}
	res := tx.Model(&model.EduConsentInvite{}).Where("id = ? AND tenant_id = ? AND status = ?", c.Param("id"), a.TenantID, "active").
		Updates(map[string]any{"status": "revoked", "updated_at": h.Svc.now()})
	if res.Error != nil {
		h.fail(c, res.Error)
		return
	}
	if res.RowsAffected == 0 {
		h.fail(c, ErrNotFound)
		return
	}
	h.Svc.audit(tx, a, "consent.online_invite.revoke", c.Param("id"), "")
	reply(c, 200, gin.H{"revoked": true}, "ok")
}

// confirmGuardianConsent is mounted outside the tenant-scoped middleware: the
// guardian does not need a platform account. The invite is located by id in
// system context, the secret is checked in constant time, and the write runs
// in TenantTx(invite tenant). Invalid, expired, used or revoked tokens all
// answer the same 404; the response never names the student.
func (h *API) confirmGuardianConsent(c *gin.Context) {
	var in struct {
		Token  string `json:"token"`
		Status string `json:"status"`
		Ack    bool   `json:"ack"`
	}
	if c.ShouldBindJSON(&in) != nil {
		reply(c, 400, nil, "token, status and ack required")
		return
	}
	if !in.Ack {
		reply(c, 400, nil, "ack must be true to confirm the notice")
		return
	}
	if in.Status != "granted" && in.Status != "withdrawn" {
		reply(c, 400, nil, "status (granted|withdrawn) required")
		return
	}
	id, secret, found := strings.Cut(strings.TrimSpace(in.Token), ".")
	if !found || !strings.HasPrefix(id, "ect_") || len(secret) != consentTokenBytes*2 {
		reply(c, 404, nil, "invalid or expired token")
		return
	}
	var inv model.EduConsentInvite
	if err := h.DB.First(&inv, "id = ?", id).Error; err != nil ||
		subtle.ConstantTimeCompare([]byte(inv.TokenHash), []byte(secretHash(secret))) != 1 {
		reply(c, 404, nil, "invalid or expired token")
		return
	}
	want := "grant"
	if in.Status == "withdrawn" {
		want = "withdraw"
	}
	now := h.Svc.now()
	if inv.Purpose != want || inv.Status != "active" || !inv.ExpiresAt.After(now) {
		reply(c, 404, nil, "invalid or expired token")
		return
	}
	err := flow.TenantTx(h.DB, inv.TenantID, func(tx *gorm.DB) error {
		res := tx.Model(&model.EduConsentInvite{}).
			Where("id = ? AND tenant_id = ? AND status = ? AND expires_at > ?", inv.ID, inv.TenantID, "active", now).
			Updates(map[string]any{"status": "used", "used_at": now, "updated_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrNotFound
		}
		if consentMode(tx, inv.TenantID) == "not_applicable" {
			return ErrNotFound
		}
		ev := "online:" + inv.ID
		actor := Actor{UserID: "guardian:online", TenantID: inv.TenantID}
		var rec model.EduConsentRecord
		if tx.First(&rec, "tenant_id = ? AND student_user_id = ? AND kind = ?", inv.TenantID, inv.StudentUserID, "guardian").Error != nil {
			if in.Status != "granted" {
				return ErrNotFound
			}
			rec = model.EduConsentRecord{
				ID: flow.NewID("ecr_"), TenantID: inv.TenantID, StudentUserID: inv.StudentUserID,
				Kind: "guardian", Status: in.Status, EvidenceRef: ev, RecordedBy: actor.UserID,
				CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Create(&rec).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&model.EduConsentRecord{}).Where("id = ?", rec.ID).
				Updates(map[string]any{"status": in.Status, "evidence_ref": ev, "recorded_by": actor.UserID, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		h.Svc.audit(tx, actor, "consent."+in.Status, inv.StudentUserID, ev)
		return nil
	})
	if err != nil {
		reply(c, 404, nil, "invalid or expired token")
		return
	}
	reply(c, 200, gin.H{"status": in.Status, "noticeVersion": inv.NoticeVersion, "channel": "online"}, "ok")
}
