package logic

import (
	"testing"

	"aiworkc/internal/domain"
	"aiworkc/internal/model"
)

func TestFromWhitelistItemRequiresKindKey(t *testing.T) {
	if _, err := fromWhitelistItem(&domain.WhitelistItem{Kind: model.WhitelistFileExt}); err == nil {
		t.Fatal("key required")
	}
	if _, err := fromWhitelistItem(&domain.WhitelistItem{Kind: "nope", Key: ".pdf"}); err == nil {
		t.Fatal("kind required")
	}
}

func TestFromWhitelistItemFileExtNeedsMagic(t *testing.T) {
	if _, err := fromWhitelistItem(&domain.WhitelistItem{Kind: model.WhitelistFileExt, Key: "xlsx"}); err == nil {
		t.Fatal("magic required")
	}
	got, err := fromWhitelistItem(&domain.WhitelistItem{Kind: model.WhitelistFileExt, Key: "xlsx", Magic: "zip"})
	if err != nil || got.Key != "xlsx" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestFromWhitelistItemAnalysisKinds(t *testing.T) {
	dep, err := fromWhitelistItem(&domain.WhitelistItem{Kind: model.WhitelistAnalysisDep, Key: "dep1"})
	if err != nil || dep.Kind != model.WhitelistAnalysisDep {
		t.Fatalf("%+v %v", dep, err)
	}
	user, err := fromWhitelistItem(&domain.WhitelistItem{Kind: model.WhitelistAnalysisUser, Key: "u1"})
	if err != nil || user.Kind != model.WhitelistAnalysisUser {
		t.Fatalf("%+v %v", user, err)
	}
}
