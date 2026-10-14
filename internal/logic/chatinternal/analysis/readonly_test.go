package analysis

import "testing"

func TestIdentName(t *testing.T) {
	if !identName.MatchString("budget") {
		t.Fatal("budget should be a valid ident")
	}
	if identName.MatchString("budget;drop") || identName.MatchString("`budget`") || identName.MatchString("") {
		t.Fatal("must reject unsafe table names")
	}
}
