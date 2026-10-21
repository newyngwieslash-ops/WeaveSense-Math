package flow

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"yingce/backend/internal/model"
)

func TestCreateOrganizationRLSPostgres(t *testing.T) {
	db := pgTestDB(t)
	var unsafeRoles int64
	if err := db.Raw("SELECT count(*) FROM pg_roles WHERE rolname IN (current_user, session_user) AND (rolsuper OR rolbypassrls)").Scan(&unsafeRoles).Error; err != nil || unsafeRoles != 0 {
		t.Fatalf("requires non-superuser without BYPASSRLS: %d %v", unsafeRoles, err)
	}
	var protected bool
	if err := db.Raw("SELECT relrowsecurity AND relforcerowsecurity FROM pg_class WHERE oid='flow_billing_accounts'::regclass").Scan(&protected).Error; err != nil || !protected {
		t.Fatalf("billing RLS must be enabled and forced: %v", err)
	}
	for _, id := range []string{"org_alice", "org_bob"} {
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
	}
	current := func(tx *gorm.DB) string {
		var value string
		if err := tx.Raw("SELECT coalesce(current_setting('app.tenant_ids',true),'')").Scan(&value).Error; err != nil {
			t.Fatal(err)
		}
		return value
	}
	count := func(tx *gorm.DB, table, field, value string) int64 {
		var n int64
		if err := tx.Table(table).Where(field+" = ?", value).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	t.Run("HTTP", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		user := func(*gin.Context) (*model.User, error) { return &model.User{ID: "org_alice"}, nil }
		api := r.Group("/api", TenantMiddleware(db, user))
		RegisterP5Routes(api, db, Config{}, user, nil)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/api/intelligence/orgs", strings.NewReader(`{"name":"http-created"}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			if count(db, "flow_tenants", "name", "http-created") != 0 {
				t.Error("failed HTTP request left organization")
			}
			t.Fatalf("create organization HTTP=%d body=%s", w.Code, w.Body.String())
		}
		var receipt struct {
			Data struct {
				Tenant    model.Tenant    `json:"tenant"`
				Workspace model.Workspace `json:"workspace"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &receipt); err != nil {
			t.Fatal(err)
		}
		tn, ws := receipt.Data.Tenant, receipt.Data.Workspace
		var account model.FlowBillingAccount
		if err := db.First(&account, "tenant_id = ?", tn.ID).Error; err != nil {
			t.Fatal(err)
		}
		if tn.OwnerUserID != "org_alice" || ws.TenantID != tn.ID || account.PayerUserID != "org_alice" || account.Kind != "shared" || account.OwnerType != "organization" {
			t.Fatal("organization ownership/billing mismatch")
		}
		s, err := ResolveScope(db, "org_alice", ws.ID)
		if err != nil || s.Role != "admin" {
			t.Fatalf("owner workspace membership missing: %v", err)
		}
	})
	t.Run("nested-context-and-isolation", func(t *testing.T) {
		err := UserTx(db, "org_alice", func(tx *gorm.DB) error {
			before := current(tx)
			tn, _, err := CreateOrganization(tx, "org_alice", "nested-created")
			if err != nil {
				return err
			}
			if current(tx) != before {
				return fmt.Errorf("caller context changed")
			}
			// Neither the new tenant nor an unrelated tenant may leak into the caller's old scope.
			if count(tx, "flow_billing_accounts", "tenant_id", tn.ID) != 0 || count(tx, "flow_billing_accounts", "tenant_id", model.PersonalTenantID("org_bob")) != 0 {
				return fmt.Errorf("foreign billing visible")
			}
			if err := tx.SavePoint("foreign_write").Error; err != nil {
				return err
			}
			foreignErr := tx.Create(&model.FlowBudgetReservation{ID: NewID("rsv_"), TenantID: model.PersonalTenantID("org_bob"), AccountID: "foreign", IdempotencyKey: "foreign", Status: "reserved"}).Error
			if err := tx.RollbackTo("foreign_write").Error; err != nil {
				return err
			}
			if foreignErr == nil || !strings.Contains(foreignErr.Error(), "42501") {
				return fmt.Errorf("foreign write not blocked: %v", foreignErr)
			}
			if current(tx) != before || count(tx, "flow_billing_accounts", "tenant_id", model.PersonalTenantID("org_alice")) != 1 {
				return fmt.Errorf("outer transaction no longer usable")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("failure-rolls-back-and-restores", func(t *testing.T) {
		var failedTenant, failedWorkspace string
		// New billing inserts must run only in that new tenant, never in system scope.
		for _, sql := range []string{
			`CREATE FUNCTION fail_org_billing() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF current_setting('app.tenant_ids',true) IS DISTINCT FROM NEW.tenant_id THEN RAISE EXCEPTION 'incorrect initialization scope'; END IF; IF (SELECT name FROM flow_tenants WHERE id=NEW.tenant_id)='injected-failure' THEN RAISE EXCEPTION 'injected billing failure'; END IF; RETURN NEW; END $$`,
			`CREATE TRIGGER org_billing_guard BEFORE INSERT ON flow_billing_accounts FOR EACH ROW EXECUTE FUNCTION fail_org_billing()`,
		} {
			if err := db.Exec(sql).Error; err != nil {
				t.Fatal(err)
			}
		}
		err := UserTx(db, "org_alice", func(tx *gorm.DB) error {
			before := current(tx)
			tn, ws, err := CreateOrganization(tx, "org_alice", "injected-failure")
			failedTenant, failedWorkspace = tn.ID, ws.ID
			if err == nil || !strings.Contains(err.Error(), "injected billing failure") {
				return fmt.Errorf("expected injected failure: %v", err)
			}
			if current(tx) != before {
				return fmt.Errorf("failed creation leaked context")
			}
			for table, field := range map[string]string{"flow_tenants": "id", "flow_memberships": "tenant_id", "flow_workspaces": "tenant_id", "flow_billing_accounts": "tenant_id"} {
				if count(tx, table, field, tn.ID) != 0 {
					return fmt.Errorf("failed creation left %s", table)
				}
			}
			if count(tx, "flow_workspace_memberships", "workspace_id", ws.ID) != 0 {
				return fmt.Errorf("failed creation left workspace membership")
			}
			if count(tx, "flow_billing_accounts", "tenant_id", model.PersonalTenantID("org_alice")) != 1 {
				return fmt.Errorf("outer transaction unusable")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		for table, field := range map[string]string{"flow_tenants": "id", "flow_memberships": "tenant_id", "flow_workspaces": "tenant_id", "flow_billing_accounts": "tenant_id"} {
			if count(db, table, field, failedTenant) != 0 {
				t.Fatalf("failed creation left %s outside caller scope", table)
			}
		}
		if count(db, "flow_workspace_memberships", "workspace_id", failedWorkspace) != 0 {
			t.Fatal("failed creation left workspace membership outside caller scope")
		}
	})
	t.Run("independent-call", func(t *testing.T) {
		_, _, err := CreateOrganization(db, "org_alice", "independent-created")
		if err != nil {
			t.Fatal(err)
		}
		if current(db) != "" {
			t.Fatal("connection retained organization context")
		}
	})
}
