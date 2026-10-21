package flow

import (
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"yingce/backend/internal/model"
)

func memberControlOrg(t *testing.T, db *gorm.DB) Scope {
	t.Helper()
	_, w, err := CreateOrganization(db, "u_alice", "member controls")
	if err != nil {
		t.Fatal(err)
	}
	s, err := ResolveScope(db, "u_alice", w.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := CreateInvite(db, s, "u_alice", "bob@example.invalid", "admin", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcceptInvite(db, "u_bob", token); err != nil {
		t.Fatal(err)
	}
	if err := SetMemberRole(db, s, "u_bob", "admin"); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestMemberControlsOwner(t *testing.T) {
	db := testDB(t)
	s := memberControlOrg(t, db)
	if err := RemoveMember(db, s, "u_alice"); !errors.Is(err, ErrForbidden) {
		t.Errorf("owner revoke must be forbidden, got %v", err)
	}
	if _, err := ResolveScope(db, "u_alice", s.WorkspaceID); err != nil {
		t.Errorf("owner lost workspace access: %v", err)
	}
}

func TestMemberControlsSelfAndLastAdmin(t *testing.T) {
	for _, last := range []bool{false, true} {
		t.Run(map[bool]string{false: "other_owner", true: "last_admin"}[last], func(t *testing.T) {
			db := testDB(t)
			s := memberControlOrg(t, db)
			bob, err := ResolveScope(db, "u_bob", s.WorkspaceID)
			if err != nil {
				t.Fatal(err)
			}
			if last {
				if err := db.Model(&model.Membership{}).Where("tenant_id = ? AND user_id = ?", s.TenantID, "u_alice").Update("status", "removed").Error; err != nil {
					t.Fatal(err)
				}
			}
			err = RemoveMember(db, bob, "u_bob")
			if last {
				if !errors.Is(err, ErrForbidden) {
					t.Fatalf("last admin revoked: %v", err)
				}
				if _, err := ResolveScope(db, "u_bob", s.WorkspaceID); err != nil {
					t.Fatal(err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := ResolveScope(db, "u_bob", s.WorkspaceID); !errors.Is(err, ErrForbidden) {
					t.Fatalf("revoked member access: %v", err)
				}
				if err := RemoveMember(db, s, "u_bob"); !errors.Is(err, ErrNotFound) {
					t.Fatalf("repeat revoke: %v", err)
				}
			}
		})
	}
}

func TestMemberControlsScopeAndRollback(t *testing.T) {
	db := testDB(t)
	s := memberControlOrg(t, db)
	for _, role := range []string{"editor", "analyst", "viewer", "unknown"} {
		other := s
		other.Role = role
		if err := RemoveMember(db, other, "u_bob"); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s revoke: %v", role, err)
		}
	}
	other, err := ResolveScope(db, "u_alice", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := RemoveMember(db, other, "u_bob"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross tenant: %v", err)
	}
	w := model.Workspace{ID: NewID("ws_"), TenantID: s.TenantID, Slug: "second", Name: "second"}
	if err := db.Create(&w).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.WorkspaceMembership{ID: NewID("wm_"), WorkspaceID: w.ID, UserID: "u_bob", Role: "viewer"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TRIGGER reject_member_delete BEFORE DELETE ON flow_workspace_memberships BEGIN SELECT RAISE(ABORT, 'injected member delete failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := RemoveMember(db, s, "u_bob"); err == nil {
		t.Fatal("delete failure reported success")
	}
	if _, err := ResolveScope(db, "u_bob", w.ID); err != nil {
		t.Fatalf("failed removal changed access: %v", err)
	}
	if err := db.Exec("DROP TRIGGER reject_member_delete").Error; err != nil {
		t.Fatal(err)
	}
	if err := RemoveMember(db, s, "u_bob"); err != nil {
		t.Fatal(err)
	}
	for _, ws := range []string{s.WorkspaceID, w.ID} {
		if _, err := ResolveScope(db, "u_bob", ws); !errors.Is(err, ErrForbidden) {
			t.Fatalf("workspace retained: %s %v", ws, err)
		}
	}
	if _, err := ResolveScope(db, "u_bob", ""); err != nil {
		t.Fatalf("personal access changed: %v", err)
	}
}

func TestMemberControlsCountError(t *testing.T) {
	for _, action := range []string{"remove", "demote"} {
		t.Run(action, func(t *testing.T) {
			db := testDB(t)
			s := memberControlOrg(t, db)
			injected := errors.New("injected member count failure")
			if err := db.Callback().Query().After("gorm:query").Register("member_count_failure", func(q *gorm.DB) {
				if q.Statement.Table == "flow_memberships" && strings.Contains(q.Statement.SQL.String(), "count(*)") {
					q.AddError(injected)
				}
			}); err != nil {
				t.Fatal(err)
			}
			var err error
			if action == "remove" {
				err = RemoveMember(db, s, "u_bob")
			} else {
				err = SetMemberRole(db, s, "u_bob", "viewer")
			}
			if !errors.Is(err, injected) {
				t.Fatalf("count failure lost: %v", err)
			}
			var m model.Membership
			if err := db.First(&m, "tenant_id = ? AND user_id = ?", s.TenantID, "u_bob").Error; err != nil {
				t.Fatal(err)
			}
			if m.Role != "admin" || m.Status != "active" {
				t.Fatalf("failed count changed member: %+v", m)
			}
		})
	}
}
