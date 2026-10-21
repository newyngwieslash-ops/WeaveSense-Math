package flow

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"yingce/backend/internal/model"
)

func TestAcceptInviteRLSPostgres(t *testing.T) {
	db := pgTestDB(t)
	var unsafe, protected int64
	if err := db.Raw("SELECT count(*) FROM pg_roles WHERE rolname IN (current_user,session_user) AND (rolsuper OR rolbypassrls)").Scan(&unsafe).Error; err != nil || unsafe != 0 {
		t.Fatalf("requires non-superuser without BYPASSRLS: %v", err)
	}
	if err := db.Raw("SELECT count(*) FROM pg_class WHERE relname='flow_invites' AND relrowsecurity AND relforcerowsecurity").Scan(&protected).Error; err != nil || protected != 1 {
		t.Fatalf("invites must have FORCE RLS: %v", err)
	}
	newUser := func() string {
		id := NewID("u_")
		if err := db.Create(&model.User{ID: id, Username: id}).Error; err != nil {
			t.Fatal(err)
		}
		s, err := ResolveScope(db, id, "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := (Billing{DB: db}).Account(db, s.TenantID); err != nil {
			t.Fatal(err)
		}
		return id
	}
	owner := newUser()
	newInvite := func(role string) (model.FlowInvite, string) {
		tn, ws, err := CreateOrganization(db, owner, "invite regression")
		if err != nil {
			t.Fatal(err)
		}
		inv, token, err := CreateInvite(db, Scope{TenantID: tn.ID, WorkspaceID: ws.ID, Role: "admin"}, owner, "test@example.invalid", role, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		return inv, token
	}
	current := func(tx *gorm.DB) string {
		var scope string
		if err := tx.Raw("SELECT coalesce(current_setting('app.tenant_ids',true),'')").Scan(&scope).Error; err != nil {
			t.Fatal(err)
		}
		return scope
	}
	count := func(tx *gorm.DB, table, field, value string) int64 {
		var n int64
		if err := tx.Table(table).Where(field+" = ?", value).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	// Every redemption write must use the invitation's exact tenant, never system scope.
	for _, stmt := range []string{
		`CREATE FUNCTION check_invite_write_scope() RETURNS trigger LANGUAGE plpgsql AS $$ DECLARE target text; BEGIN IF TG_TABLE_NAME='flow_workspace_memberships' THEN SELECT tenant_id INTO target FROM flow_workspaces WHERE id=NEW.workspace_id; ELSE target:=NEW.tenant_id; END IF; IF current_setting('app.tenant_ids',true) IS DISTINCT FROM target THEN RAISE EXCEPTION 'incorrect redemption scope'; END IF; RETURN NEW; END $$`,
		`CREATE TRIGGER invite_scope_guard BEFORE UPDATE ON flow_invites FOR EACH ROW WHEN (NEW.accepted_at IS NOT NULL) EXECUTE FUNCTION check_invite_write_scope()`,
		`CREATE TRIGGER invite_tenant_member_scope_guard BEFORE INSERT ON flow_memberships FOR EACH ROW WHEN (NEW.role='member') EXECUTE FUNCTION check_invite_write_scope()`,
		`CREATE TRIGGER invite_workspace_member_scope_guard BEFORE INSERT ON flow_workspace_memberships FOR EACH ROW WHEN (NEW.role<>'admin') EXECUTE FUNCTION check_invite_write_scope()`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Run("HTTP", func(t *testing.T) {
		uid := newUser()
		inv, token := newInvite("viewer")
		gin.SetMode(gin.TestMode)
		r := gin.New()
		user := func(*gin.Context) (*model.User, error) { return &model.User{ID: uid}, nil }
		api := r.Group("/api", TenantMiddleware(db, user))
		RegisterP5Routes(api, db, Config{}, user, nil)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/intelligence/invites/accept", strings.NewReader(`{"token":"`+token+`"}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			if count(db, "flow_workspace_memberships", "workspace_id", inv.WorkspaceID) != 1 {
				t.Error("failed HTTP accept left a member")
			}
			t.Fatalf("accept valid invite HTTP=%d body=%s", w.Code, w.Body.String())
		}
		var response struct {
			Code int   `json:"code"`
			Data Scope `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.Code != 0 || response.Data != (Scope{TenantID: inv.TenantID, WorkspaceID: inv.WorkspaceID, Role: "viewer"}) {
			t.Fatal("receipt mismatch")
		}
		s, err := ResolveScope(db, uid, inv.WorkspaceID)
		if err != nil || s.Role != "viewer" || hasScope(s.Role, "intel.sources.write") {
			t.Fatalf("viewer scope mismatch: %v", err)
		}
		if err := RemoveMember(db, Scope{TenantID: inv.TenantID, WorkspaceID: inv.WorkspaceID, Role: "admin"}, uid); err != nil {
			t.Fatal(err)
		}
		if _, err := ResolveScope(db, uid, inv.WorkspaceID); !errors.Is(err, ErrForbidden) {
			t.Fatalf("revoked access: %v", err)
		}
	})
	t.Run("nested-context-and-isolation", func(t *testing.T) {
		uid := newUser()
		inv, token := newInvite("analyst")
		if err := UserTx(db, uid, func(tx *gorm.DB) error {
			before := current(tx)
			s, err := AcceptInvite(tx, uid, token)
			if err != nil {
				return err
			}
			if s.Role != "analyst" || current(tx) != before {
				return fmt.Errorf("scope/role not restored")
			}
			if count(tx, "flow_billing_accounts", "tenant_id", inv.TenantID) != 0 || count(tx, "flow_invites", "id", inv.ID) != 0 {
				return fmt.Errorf("target scope leaked")
			}
			if count(tx, "flow_billing_accounts", "tenant_id", model.PersonalTenantID(uid)) != 1 {
				return fmt.Errorf("old scope unusable")
			}
			if _, err := AcceptInvite(tx, uid, token); !errors.Is(err, ErrInviteInvalid) {
				return fmt.Errorf("repeat token accepted: %v", err)
			}
			if current(tx) != before {
				return fmt.Errorf("rejection leaked scope")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if s, err := ResolveScope(db, uid, inv.WorkspaceID); err != nil || s.Role != "analyst" {
			t.Fatalf("membership not committed: %v", err)
		}
	})
	t.Run("invalid-expired-revoked", func(t *testing.T) {
		uid := newUser()
		for _, kind := range []string{"invalid", "expired", "revoked"} {
			inv, token := newInvite("editor")
			switch kind {
			case "invalid":
				token = "unknown-token"
			case "expired":
				if err := db.Model(&inv).Update("expires_at", time.Now().Add(-time.Hour)).Error; err != nil {
					t.Fatal(err)
				}
			case "revoked":
				if err := db.Model(&inv).Update("revoked_at", time.Now()).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := UserTx(db, uid, func(tx *gorm.DB) error {
				before := current(tx)
				if _, err := AcceptInvite(tx, uid, token); !errors.Is(err, ErrInviteInvalid) {
					return fmt.Errorf("%s accepted: %v", kind, err)
				}
				if current(tx) != before {
					return fmt.Errorf("invalid token leaked scope")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if count(db, "flow_workspace_memberships", "workspace_id", inv.WorkspaceID) != 1 {
				t.Fatal("invalid token added member")
			}
		}
	})
	t.Run("write-failure-rolls-back-and-restores", func(t *testing.T) {
		uid := newUser()
		inv, token := newInvite("viewer")
		for _, stmt := range []string{
			`CREATE FUNCTION fail_invite_member() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.user_id='` + uid + `' THEN IF current_setting('app.tenant_ids',true) IS DISTINCT FROM (SELECT tenant_id FROM flow_workspaces WHERE id=NEW.workspace_id) THEN RAISE EXCEPTION 'incorrect member scope'; END IF; RAISE EXCEPTION 'injected member failure'; END IF; RETURN NEW; END $$`,
			`CREATE TRIGGER invite_member_guard BEFORE INSERT ON flow_workspace_memberships FOR EACH ROW EXECUTE FUNCTION fail_invite_member()`,
		} {
			if err := db.Exec(stmt).Error; err != nil {
				t.Fatal(err)
			}
		}
		defer db.Exec("DROP TRIGGER invite_member_guard ON flow_workspace_memberships; DROP FUNCTION fail_invite_member()")
		if err := UserTx(db, uid, func(tx *gorm.DB) error {
			before := current(tx)
			if _, err := AcceptInvite(tx, uid, token); err == nil || !strings.Contains(err.Error(), "injected member failure") {
				return fmt.Errorf("expected injected failure: %v", err)
			}
			if current(tx) != before || count(tx, "flow_billing_accounts", "tenant_id", model.PersonalTenantID(uid)) != 1 {
				return fmt.Errorf("failed accept damaged caller scope")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		var stored model.FlowInvite
		if err := db.First(&stored, "id = ?", inv.ID).Error; err != nil {
			t.Fatal(err)
		}
		if stored.AcceptedAt != nil || count(db, "flow_memberships", "tenant_id", inv.TenantID) != 1 || count(db, "flow_workspace_memberships", "workspace_id", inv.WorkspaceID) != 1 {
			t.Fatal("failed accept left consumed invite or member")
		}
	})
	t.Run("concurrent-single-use", func(t *testing.T) {
		inv, token := newInvite("editor")
		uids := []string{newUser(), newUser()}
		start := make(chan struct{})
		results := make(chan error, 2)
		for _, uid := range uids {
			go func(uid string) {
				<-start
				results <- UserTx(db, uid, func(tx *gorm.DB) error { _, err := AcceptInvite(tx, uid, token); return err })
			}(uid)
		}
		close(start)
		success, invalid := 0, 0
		for range uids {
			err := <-results
			if err == nil {
				success++
			} else if errors.Is(err, ErrInviteInvalid) {
				invalid++
			} else {
				t.Fatal(err)
			}
		}
		if success != 1 || invalid != 1 || count(db, "flow_memberships", "tenant_id", inv.TenantID) != 2 || count(db, "flow_workspace_memberships", "workspace_id", inv.WorkspaceID) != 2 {
			t.Fatal("token was not single use")
		}
	})
	t.Run("independent-call", func(t *testing.T) {
		uid := newUser()
		inv, token := newInvite("editor")
		if _, err := AcceptInvite(db, uid, token); err != nil {
			t.Fatal(err)
		}
		if current(db) != "" {
			t.Fatal("connection retained target scope")
		}
		if s, err := ResolveScope(db, uid, inv.WorkspaceID); err != nil || s.Role != "editor" {
			t.Fatalf("independent membership: %v", err)
		}
	})
}
