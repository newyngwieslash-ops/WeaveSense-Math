package flow

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"yingce/backend/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrInviteInvalid = errors.New("flow: invite invalid, expired or already used")

// CreateOrganization creates an organization tenant with the caller as owner,
// a default workspace (caller admin) and a shared billing account paid by the owner.
func CreateOrganization(db *gorm.DB, ownerID, name string) (model.Tenant, model.Workspace, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return model.Tenant{}, model.Workspace{}, errors.New("flow: organization name required")
	}
	t := model.Tenant{ID: NewID("tn_"), Kind: "organization", Name: name, OwnerUserID: ownerID, Status: "active"}
	w := model.Workspace{ID: NewID("ws_"), TenantID: t.ID, Slug: "default", Name: name}
	err := db.Transaction(func(tx *gorm.DB) error {
		var previousScope string
		if tx.Dialector.Name() == "postgres" {
			if err := tx.Raw("SELECT coalesce(current_setting('app.tenant_ids', true), '')").Scan(&previousScope).Error; err != nil {
				return err
			}
			// Bootstrap only this server-generated tenant; never enter system scope.
			if err := tx.Exec("SELECT set_config('app.tenant_ids', ?, true)", t.ID).Error; err != nil {
				return err
			}
		}
		for _, r := range []any{&t, &model.Membership{ID: NewID("mb_"), TenantID: t.ID, UserID: ownerID, Role: "owner", Status: "active"}, &w,
			&model.WorkspaceMembership{ID: NewID("wm_"), WorkspaceID: w.ID, UserID: ownerID, Role: "admin"}} {
			if err := tx.Create(r).Error; err != nil {
				return err
			}
		}
		if _, err := (Billing{DB: tx}).Account(tx, t.ID); err != nil {
			return err
		}
		if tx.Dialector.Name() == "postgres" {
			return tx.Exec("SELECT set_config('app.tenant_ids', ?, true)", previousScope).Error
		}
		return nil
	})
	return t, w, err
}

func hashToken(tok string) string {
	s := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(s[:])
}

// CreateInvite (workspace admin of an organization tenant) returns the one-time token.
func CreateInvite(db *gorm.DB, scope Scope, inviterID, email, role string, ttl time.Duration) (model.FlowInvite, string, error) {
	if scope.Role != "admin" && scope.Role != "owner" {
		return model.FlowInvite{}, "", ErrForbidden
	}
	var t model.Tenant
	if err := db.First(&t, "id = ?", scope.TenantID).Error; err != nil || t.Kind != "organization" {
		return model.FlowInvite{}, "", errors.New("flow: invites are only for organization tenants")
	}
	switch role {
	case "admin", "editor", "analyst", "viewer":
	default:
		return model.FlowInvite{}, "", errors.New("flow: invalid role")
	}
	var b [24]byte
	_, _ = rand.Read(b[:])
	tok := hex.EncodeToString(b[:])
	inv := model.FlowInvite{ID: NewID("inv_"), TenantID: scope.TenantID, WorkspaceID: scope.WorkspaceID, Email: strings.TrimSpace(email), Role: role,
		TokenHash: hashToken(tok), InvitedBy: inviterID, ExpiresAt: time.Now().UTC().Add(ttl), CreatedAt: time.Now().UTC()}
	return inv, tok, db.Create(&inv).Error
}

// AcceptInvite joins the user to the tenant and workspace. Single use.
func AcceptInvite(db *gorm.DB, userID, token string) (Scope, error) {
	if userID == "" || token == "" {
		return Scope{}, ErrInviteInvalid
	}
	var out Scope
	err := db.Transaction(func(tx *gorm.DB) error {
		var previousScope string
		if tx.Dialector.Name() == "postgres" {
			if err := tx.Raw("SELECT coalesce(current_setting('app.tenant_ids', true), '')").Scan(&previousScope).Error; err != nil {
				return err
			}
		}
		var inv model.FlowInvite
		// The bearer token permits exactly one cross-tenant lookup, never member writes.
		if err := withSystemContext(tx, func(lookup *gorm.DB) error {
			return lookup.Clauses(clause.Locking{Strength: "UPDATE"}).First(&inv, "token_hash = ?", hashToken(token)).Error
		}); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInviteInvalid
			}
			return err
		}
		now := time.Now().UTC()
		if inv.AcceptedAt != nil || inv.RevokedAt != nil || now.After(inv.ExpiresAt) {
			return ErrInviteInvalid
		}
		if tx.Dialector.Name() == "postgres" {
			if err := tx.Exec("SELECT set_config('app.tenant_ids', ?, true)", inv.TenantID).Error; err != nil {
				return err
			}
		}
		res := tx.Model(&model.FlowInvite{}).Where("id = ? AND accepted_at IS NULL", inv.ID).Updates(map[string]any{"accepted_by": userID, "accepted_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrInviteInvalid
		}
		var m model.Membership
		if err := tx.First(&m, "tenant_id = ? AND user_id = ?", inv.TenantID, userID).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err := tx.Create(&model.Membership{ID: NewID("mb_"), TenantID: inv.TenantID, UserID: userID, Role: "member", Status: "active"}).Error; err != nil {
				return err
			}
		}
		var wm model.WorkspaceMembership
		if err := tx.First(&wm, "workspace_id = ? AND user_id = ?", inv.WorkspaceID, userID).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err := tx.Create(&model.WorkspaceMembership{ID: NewID("wm_"), WorkspaceID: inv.WorkspaceID, UserID: userID, Role: inv.Role}).Error; err != nil {
				return err
			}
		}
		out = Scope{TenantID: inv.TenantID, WorkspaceID: inv.WorkspaceID, Role: inv.Role}
		if tx.Dialector.Name() == "postgres" {
			return tx.Exec("SELECT set_config('app.tenant_ids', ?, true)", previousScope).Error
		}
		return nil
	})
	return out, err
}

// RemoveMember deactivates a tenant membership (ongoing authorization checks then fail).
func RemoveMember(db *gorm.DB, scope Scope, userID string) error {
	if scope.Role != "admin" && scope.Role != "owner" {
		return ErrForbidden
	}
	return db.Transaction(func(tx *gorm.DB) error {
		// Serialize with SetMemberRole before checking the tenant's remaining admins.
		var tenant model.Tenant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&tenant, "id = ?", scope.TenantID).Error; err != nil {
			return err
		}
		var m model.Membership
		if err := tx.First(&m, "tenant_id = ? AND user_id = ? AND status = ?", scope.TenantID, userID, "active").Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if m.Role == "owner" {
			return ErrForbidden
		}
		if m.Role == "admin" {
			var n int64
			if err := tx.Model(&model.Membership{}).Where("tenant_id = ? AND status = ? AND role IN ?", scope.TenantID, "active", []string{"admin", "owner"}).Count(&n).Error; err != nil {
				return err
			}
			if n <= 1 {
				return ErrForbidden
			}
		}
		if err := tx.Model(&m).Update("status", "removed").Error; err != nil {
			return err
		}
		return tx.Where("user_id = ? AND workspace_id IN (?)", userID, tx.Model(&model.Workspace{}).Select("id").Where("tenant_id = ?", scope.TenantID)).Delete(&model.WorkspaceMembership{}).Error
	})
}

// AuthorizeGrant re-checks that a schedule's granting actor ("user:<id>") still
// has the scope in that workspace (membership active, tenant active, role allows).
func AuthorizeGrant(db *gorm.DB, actor, tenantID, workspaceID, scope string) error {
	userID, ok := strings.CutPrefix(actor, "user:")
	if !ok || userID == "" {
		return ErrForbidden
	}
	s, err := ResolveScope(db, userID, workspaceID)
	if err != nil || s.TenantID != tenantID || !hasScope(s.Role, scope) {
		return ErrForbidden
	}
	return nil
}
