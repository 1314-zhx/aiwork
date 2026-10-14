package sqlguard

import (
	"errors"
	"strings"
	"testing"
)

func TestParseRejectsUnionAndQualifiedTable(t *testing.T) {
	if _, err := Parse("SELECT title FROM budget UNION SELECT title FROM budget"); err == nil {
		t.Fatal("expected union reject")
	}
	if _, err := Parse("SELECT title FROM mysql.user"); err == nil {
		t.Fatal("expected qualified table reject")
	}
}

func TestForceLimitCapsAndAdds(t *testing.T) {
	out, err := ForceLimit("SELECT title FROM budget", 100)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(out), "limit") {
		t.Fatalf("missing limit: %s", out)
	}
	out, err = ForceLimit("SELECT title FROM budget LIMIT 9999", 100)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "9999") {
		t.Fatalf("limit not capped: %s", out)
	}
}

func TestInjectTenantFailClosed(t *testing.T) {
	p := DefaultPolicy()
	_, err := InjectTenant("SELECT title FROM budget", p, Tenant{UserID: ""})
	if !errors.Is(err, ErrNoTenant) {
		t.Fatalf("got %v", err)
	}
	out, err := InjectTenant("SELECT title FROM budget", p, Tenant{UserName: "alice", UserID: "u1"})
	if err != nil {
		t.Fatal(err)
	}
	low := strings.ToLower(out)
	if !strings.Contains(low, "user") || !strings.Contains(out, "alice") {
		t.Fatalf("tenant missing: %s", out)
	}
	out, err = InjectTenant("SELECT title FROM budget", p, Tenant{IsSystem: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(out), " user ") && strings.Contains(out, "alice") {
		t.Fatalf("system should skip tenant: %s", out)
	}
}

func TestRewriteNeverExecutesStar(t *testing.T) {
	p := DefaultPolicy()
	out, err := Rewrite("SELECT * FROM budget", p, RewriteOpts{
		TopK:   50,
		Tables: []string{"budget"},
		Tenant: Tenant{UserName: "bob"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "*") {
		t.Fatalf("star remains: %s", out)
	}
	if !strings.Contains(strings.ToLower(out), "limit") {
		t.Fatalf("no limit: %s", out)
	}
	if err := Check(out, p, []string{"budget"}); err != nil {
		t.Fatal(err)
	}
}

func TestDraftStoreRestore(t *testing.T) {
	s := NewDraftStore(0)
	s.Put(Draft{ID: "d1", UserID: "u1", SQL: "SELECT title FROM budget"})
	got, err := s.Take("u1", "d1")
	if err != nil {
		t.Fatal(err)
	}
	s.Restore(got)
	again, err := s.Take("u1", "d1")
	if err != nil || again.SQL != got.SQL {
		t.Fatalf("%+v %v", again, err)
	}
}
