package logic

import (
	"strings"
	"testing"
	"time"

	"aiworkc/internal/domain"
	"aiworkc/internal/model"
)

func TestBuildTodoFromExtractedRejectsEmptyTitle(t *testing.T) {
	_, err := buildTodoFromExtracted(&domain.Todo{Desc: "x"})
	if err == nil {
		t.Fatal("expected title parse failure")
	}
}

func TestBuildTodoFromExtractedDropsIdentityFields(t *testing.T) {
	got, err := buildTodoFromExtracted(&domain.Todo{
		ID:        "injected",
		CreatorId: "injected-user",
		Title:     " 写周报 ",
		Desc:      "周五前",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "" || got.CreatorId != "" {
		t.Fatalf("identity fields leaked: %+v", got)
	}
	if got.Title != "写周报" {
		t.Fatalf("title = %q", got.Title)
	}
}

func TestBuildApprovalFromExtractedDefaultsIncompleteLeave(t *testing.T) {
	before := time.Now().Unix()
	got, err := buildApprovalFromExtracted(&domain.Approval{
		Type:  int(model.LeaveApproval),
		Leave: &domain.Leave{StartTime: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Leave == nil || got.Leave.EndTime-got.Leave.StartTime != 24*3600 {
		t.Fatalf("want default 1-day range, got %+v", got.Leave)
	}
	if got.Leave.StartTime < before-1 {
		t.Fatalf("default start too early: %d", got.Leave.StartTime)
	}
}

func TestBuildApprovalFromExtractedKeepsBusinessFieldsOnly(t *testing.T) {
	got, err := buildApprovalFromExtracted(&domain.Approval{
		Type:   int(model.LeaveApproval),
		UserId: "injected",
		Title:  "injected-title",
		Leave: &domain.Leave{
			Type:      2,
			StartTime: 100,
			EndTime:   200,
			Reason:    "病假",
			TimeType:  1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.UserId != "" || got.Title != "" {
		t.Fatalf("identity fields leaked: %+v", got)
	}
	if got.Leave == nil || got.Leave.Reason != "病假" {
		t.Fatalf("leave fields lost: %+v", got)
	}
}

func TestSQLDraftIDFromFieldAndData(t *testing.T) {
	id, err := sqlDraftID(&domain.ChatConfirmReq{DraftId: " abc "})
	if err != nil || id != "abc" {
		t.Fatalf("field: %q %v", id, err)
	}
	id, err = sqlDraftID(&domain.ChatConfirmReq{Data: map[string]any{"draftId": "d2", "sql": "SELECT 1"}})
	if err != nil || id != "d2" {
		t.Fatalf("data: %q %v", id, err)
	}
	if _, err := sqlDraftID(&domain.ChatConfirmReq{Data: map[string]any{"sql": "SELECT 1"}}); err == nil {
		t.Fatal("sql without draftId must fail")
	}
}

func TestSQLConfirmActionOnlyConfirmOrReject(t *testing.T) {
	got, err := sqlConfirmAction(&domain.ChatConfirmReq{})
	if err != nil || got != sqlActionConfirm {
		t.Fatalf("default: %q %v", got, err)
	}
	got, err = sqlConfirmAction(&domain.ChatConfirmReq{Action: "REJECT"})
	if err != nil || got != sqlActionReject {
		t.Fatalf("reject: %q %v", got, err)
	}
	got, err = sqlConfirmAction(&domain.ChatConfirmReq{Action: "retry"})
	if err != nil || got != sqlActionRetry {
		t.Fatalf("retry: %q %v", got, err)
	}
	got, err = sqlConfirmAction(&domain.ChatConfirmReq{ChatType: int(domain.TokenRetry), DraftId: "d1"})
	if err != nil || got != sqlActionRetry {
		t.Fatalf("token retry type: %q %v", got, err)
	}
	if _, err := sqlConfirmAction(&domain.ChatConfirmReq{Action: "edit"}); err == nil {
		t.Fatal("edit must be rejected")
	}
}

func TestSQLFeedbackIgnoresClientSQL(t *testing.T) {
	fb := sqlFeedback(&domain.ChatConfirmReq{
		Feedback: "只要已完成的预算",
		Data:     map[string]any{"sql": "SELECT * FROM mysql.user", "draftId": "x"},
	})
	if fb != "只要已完成的预算" {
		t.Fatalf("feedback=%q", fb)
	}
	if strings.Contains(fb, "mysql.user") {
		t.Fatal("client SQL leaked into feedback")
	}
}
