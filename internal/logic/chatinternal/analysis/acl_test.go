package analysis

import (
	"context"
	"errors"
	"testing"

	"aiworkc/internal/model"
)

func TestCheckAnalysisACL_SystemBypass(t *testing.T) {
	err := checkAnalysisACL(context.Background(), analysisACL{
		Enabled: func(context.Context, string, string) (bool, error) {
			t.Fatal("whitelist should not be consulted for system users")
			return false, nil
		},
	}, analysisIdentity{UserID: "u1", IsSystem: true})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCheckAnalysisACL_NamedUser(t *testing.T) {
	err := checkAnalysisACL(context.Background(), analysisACL{
		Enabled: func(_ context.Context, kind, key string) (bool, error) {
			return kind == model.WhitelistAnalysisUser && key == "u1", nil
		},
	}, analysisIdentity{UserID: "u1", DepID: "d9"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCheckAnalysisACL_Department(t *testing.T) {
	err := checkAnalysisACL(context.Background(), analysisACL{
		Enabled: func(_ context.Context, kind, key string) (bool, error) {
			return kind == model.WhitelistAnalysisDep && key == "dep1", nil
		},
	}, analysisIdentity{UserID: "u2", DepID: "dep1"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCheckAnalysisACL_DenyEmptyAndUnknown(t *testing.T) {
	deny := analysisACL{Enabled: func(context.Context, string, string) (bool, error) {
		return false, nil
	}}
	if err := checkAnalysisACL(context.Background(), deny, analysisIdentity{}); err == nil {
		t.Fatal("empty identity must fail")
	}
	if err := checkAnalysisACL(context.Background(), deny, analysisIdentity{UserID: "u3"}); !errors.Is(err, ErrAnalysisForbidden) {
		t.Fatalf("no dep: %v", err)
	}
	if err := checkAnalysisACL(context.Background(), deny, analysisIdentity{UserID: "u3", DepID: "dep2"}); !errors.Is(err, ErrAnalysisForbidden) {
		t.Fatalf("unknown dep: %v", err)
	}
}

func TestCheckAnalysisACL_StoreError(t *testing.T) {
	boom := errors.New("mongo down")
	err := checkAnalysisACL(context.Background(), analysisACL{
		Enabled: func(context.Context, string, string) (bool, error) { return false, boom },
	}, analysisIdentity{UserID: "u1"})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}
