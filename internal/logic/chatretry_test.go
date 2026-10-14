package logic

import (
	"context"
	"errors"
	"testing"

	"aiworkc/internal/domain"
	"aiworkc/internal/svc"
	"aiworkc/pkg/langchain/router"
	"aiworkc/pkg/langchain/sqlguard"
	"aiworkc/pkg/langchain/tokencost"
	"aiworkc/pkg/langchain/tokenretry"
	"aiworkc/pkg/token"
)

func TestOfferSQLTokenRetryRestoresDraft(t *testing.T) {
	l := &chat{svc: &svc.ServiceContext{
		SQLDrafts:    sqlguard.NewDraftStore(0),
		TokenRetries: tokenretry.NewStore(0),
	}}
	draft := sqlguard.Draft{ID: "d1", UserID: "u1", SQL: "SELECT title FROM budget", Query: "预算"}
	resp := l.offerSQLTokenRetry("u1", "d1", tokenretry.KindSQLExecute, "", &draft)
	if resp == nil || resp.ChatType != int(domain.TokenRetry) {
		t.Fatalf("resp=%+v", resp)
	}
	data, ok := resp.Data.(domain.TokenRetryResp)
	if !ok || data.DraftId != "d1" || !data.CanRetry {
		t.Fatalf("data=%+v", resp.Data)
	}
	got, err := l.svc.SQLDrafts.Get("u1", "d1")
	if err != nil || got.SQL != draft.SQL {
		t.Fatalf("draft not restored: %+v %v", got, err)
	}
	ticket, err := l.svc.TokenRetries.Get("u1", "d1")
	if err != nil || ticket.Kind != tokenretry.KindSQLExecute {
		t.Fatalf("ticket=%+v %v", ticket, err)
	}
}

func TestTokenRetryChatPayload(t *testing.T) {
	resp := tokenRetryChat("abc")
	if resp.ChatType != int(domain.TokenRetry) {
		t.Fatalf("chatType=%d", resp.ChatType)
	}
	data := resp.Data.(domain.TokenRetryResp)
	if data.DraftId != "abc" || !data.CanRetry || data.Summary == "" {
		t.Fatalf("data=%+v", data)
	}
}

func TestOfferAnalysisRetrySavesPartial(t *testing.T) {
	l := &chat{svc: &svc.ServiceContext{TokenRetries: tokenretry.NewStore(0)}}
	ctx := context.WithValue(context.Background(), token.Identify, "u1")
	miss := router.NewMissing("file", []string{"fileId", "fileName"}, []string{"文件ID", "文件名"}, map[string]string{
		"fileName": "预算.xlsx",
		"input":    "分析这个表",
	}, "请补充文件ID或文件名。")
	resp := l.offerAnalysisRetry(ctx, &domain.ChatReq{Prompts: "分析这个表", DraftId: "e1"}, miss)
	if resp == nil || resp.ChatType != int(domain.AnalysisRetry) {
		t.Fatalf("resp=%+v", resp)
	}
	data, ok := resp.Data.(domain.AnalysisRetryResp)
	if !ok || data.EventId != "e1" || data.DraftId != "e1" || !data.CanRetry {
		t.Fatalf("data=%+v", resp.Data)
	}
	if data.Reason != "用户输入不包含文件ID，文件名等必须内容。" || data.Got["fileName"] != "预算.xlsx" {
		t.Fatalf("payload=%+v", data)
	}
	ticket, err := l.svc.TokenRetries.Get("u1", "e1")
	if err != nil || ticket.Kind != tokenretry.KindAnalysisFill || ticket.Destination != "file" {
		t.Fatalf("ticket=%+v %v", ticket, err)
	}
	if ticket.Partial["fileName"] != "预算.xlsx" || ticket.NextInputs != "分析这个表" {
		t.Fatalf("saved=%+v", ticket)
	}
}

func TestRetryAnalysisRequiresEventId(t *testing.T) {
	l := &chat{svc: &svc.ServiceContext{TokenRetries: tokenretry.NewStore(0)}}
	ctx := context.WithValue(context.Background(), token.Identify, "u1")
	if _, err := l.retryAnalysis(ctx, &domain.ChatReq{ChatType: int(domain.AnalysisRetry)}); err == nil {
		t.Fatal("eventId required")
	}
}

func TestRetryAnalysisSkipsRouterWhenIntentUnknown(t *testing.T) {
	l := &chat{svc: &svc.ServiceContext{TokenRetries: tokenretry.NewStore(0)}}
	ctx := context.WithValue(context.Background(), token.Identify, "u1")
	l.svc.TokenRetries.Put(tokenretry.Ticket{
		ID:         "e2",
		UserID:     "u1",
		Kind:       tokenretry.KindAnalysisFill,
		NextInputs: "看看数据",
		Chat:       tokenretry.ChatSnapshot{Prompts: "看看数据"},
	})
	resp, err := l.retryAnalysis(ctx, &domain.ChatReq{
		ChatType: int(domain.AnalysisRetry),
		DraftId:  "e2",
		Prompts:  "还是那些",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ChatType != int(domain.AnalysisRetry) {
		t.Fatalf("chatType=%d", resp.ChatType)
	}
	data := resp.Data.(domain.AnalysisRetryResp)
	if data.EventId != "e2" || !data.CanRetry {
		t.Fatalf("data=%+v", data)
	}
}

func TestWithAIBudgetPrechecksDailyNeed(t *testing.T) {
	d := tokencost.NewDailyLimiter(10000)
	d.Add("u1", 8600)
	l := &chat{svc: &svc.ServiceContext{TokenDaily: d}}
	ctx := context.WithValue(context.Background(), token.Identify, "u1")

	if _, err := l.withAIBudget(ctx, tokencost.RouterDailyNeed); !errors.Is(err, tokencost.ErrDailyNotEnough) {
		t.Fatalf("router: %v", err)
	}
	if _, err := l.withAIBudget(ctx, tokencost.ReActTokenLimit); !errors.Is(err, tokencost.ErrDailyNotEnough) {
		t.Fatalf("react: %v", err)
	}

	d2 := tokencost.NewDailyLimiter(10000)
	d2.Add("u2", 1000)
	l.svc.TokenDaily = d2
	ctx = context.WithValue(context.Background(), token.Identify, "u2")
	got, err := l.withAIBudget(ctx, tokencost.RouterDailyNeed)
	if err != nil {
		t.Fatal(err)
	}
	if err := tokencost.RequireDaily(got, tokencost.HandlerDailyNeed("knowledge")); err != nil {
		t.Fatal(err)
	}
	d2.Add("u2", 2001)
	if err := tokencost.RequireDaily(got, tokencost.HandlerDailyNeed("knowledge")); err != nil {
		t.Fatalf("remain 6999 should enter knowledge: %v", err)
	}
	d2.Add("u2", 4000)
	if err := tokencost.RequireDaily(got, tokencost.HandlerDailyNeed("knowledge")); !errors.Is(err, tokencost.ErrDailyNotEnough) {
		t.Fatalf("react after more usage: %v", err)
	}
}
