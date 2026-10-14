package model

import "testing"

func TestBuildSQLPolicyUsesTableRows(t *testing.T) {
	p := BuildSQLPolicy([]*Whitelist{{
		Kind:         WhitelistSQLTable,
		Key:          "orders",
		Enabled:      true,
		Columns:      []string{"title", "amount"},
		TenantColumn: "user",
	}}, nil, nil)
	if !p.AllowTable("orders") {
		t.Fatal("orders should be allowed")
	}
	if p.AllowTable("budget") {
		t.Fatal("budget should be replaced when tables exist")
	}
	if !p.AllowColumn("orders", "title") || p.AllowColumn("orders", "password") {
		t.Fatal("column allow mismatch")
	}
}

func TestBuildSQLPolicyKeepsDefaultWhenNoTables(t *testing.T) {
	p := BuildSQLPolicy(nil, nil, nil)
	if !p.AllowTable("budget") {
		t.Fatal("default budget missing")
	}
}
