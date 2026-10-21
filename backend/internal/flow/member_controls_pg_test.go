package flow

import (
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
	"yingce/backend/internal/model"
)

func TestMemberControlsPostgres(t *testing.T) {
	db := pgTestDB(t)
	var privileged bool
	if err := db.Raw("SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user").Scan(&privileged).Error; err != nil || privileged {
		t.Fatalf("ordinary role required: %v", err)
	}
	for _, uid := range []string{"u_alice", "u_bob", "u_carol"} {
		if err := db.Create(&model.User{ID: uid, Username: uid}).Error; err != nil {
			t.Fatal(err)
		}
	}
	s := memberControlOrg(t, db)
	if err := TenantTx(db, s.TenantID, func(tx *gorm.DB) error { return RemoveMember(tx, s, "u_alice") }); !errors.Is(err, ErrForbidden) {
		t.Fatalf("owner revoke: %v", err)
	}
	// Only the disposable test database receives this failure-injection trigger.
	if err := db.Exec(`CREATE FUNCTION fail_member_delete() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected member delete failure'; END $$;
CREATE TRIGGER fail_member_delete BEFORE DELETE ON flow_workspace_memberships FOR EACH ROW EXECUTE FUNCTION fail_member_delete()`).Error; err != nil {
		t.Fatal(err)
	}
	if err := TenantTx(db, s.TenantID, func(tx *gorm.DB) error { return RemoveMember(tx, s, "u_bob") }); err == nil {
		t.Fatal("injected failure reported success")
	}
	if _, err := ResolveScope(db, "u_bob", s.WorkspaceID); err != nil {
		t.Fatalf("rollback lost access: %v", err)
	}
	if err := db.Exec("DROP TRIGGER fail_member_delete ON flow_workspace_memberships").Error; err != nil {
		t.Fatal(err)
	}
	_, token, err := CreateInvite(db, s, "u_alice", "carol@example.invalid", "admin", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcceptInvite(db, "u_carol", token); err != nil {
		t.Fatal(err)
	}
	if err := SetMemberRole(db, s, "u_carol", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Membership{}).Where("tenant_id = ? AND user_id = ?", s.TenantID, "u_alice").Update("status", "removed").Error; err != nil {
		t.Fatal(err)
	}
	// Hold the common lock until both independent transactions are waiting on it.
	hold := db.Begin()
	if hold.Error != nil {
		t.Fatal(hold.Error)
	}
	defer hold.Rollback()
	if err := hold.Exec("SELECT id FROM flow_tenants WHERE id = ? FOR UPDATE", s.TenantID).Error; err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 2)
	go func() {
		result <- TenantTx(db, s.TenantID, func(tx *gorm.DB) error { return RemoveMember(tx, s, "u_bob") })
	}()
	go func() {
		result <- TenantTx(db, s.TenantID, func(tx *gorm.DB) error { return SetMemberRole(tx, s, "u_carol", "viewer") })
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting int64
		if err := db.Raw("SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock' AND query LIKE '%FOR UPDATE%'").Scan(&waiting).Error; err != nil {
			t.Fatal(err)
		}
		if waiting >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("both member mutations must wait on the common tenant lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := hold.Commit().Error; err != nil {
		t.Fatal(err)
	}
	var passed, denied int
	for i := 0; i < 2; i++ {
		select {
		case err := <-result:
			if err == nil {
				passed++
			} else if errors.Is(err, ErrForbidden) {
				denied++
			} else {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent mutation timed out")
		}
	}
	if passed != 1 || denied != 1 {
		t.Fatalf("race results success=%d denied=%d", passed, denied)
	}
	var remaining int64
	if err := db.Model(&model.Membership{}).Where("tenant_id = ? AND status = 'active' AND role IN ('owner','admin')", s.TenantID).Count(&remaining).Error; err != nil || remaining != 1 {
		t.Fatalf("remaining=%d error=%v", remaining, err)
	}
}
