package logic

import (
	"aiworkc/internal/domain"
	"aiworkc/pkg/langchain"
	"testing"
)

func TestResolveChatLogSummary(t *testing.T) {
	uid := "user-a"
	peer := "user-b"
	req := &domain.ChatLogSummaryReq{PeerUserId: "  " + peer + "  ", StartTime: 100, EndTime: 200, Goal: "提取待办"}

	gotPeer, cid, err := resolveChatLogSummary(uid, req)
	if err != nil {
		t.Fatal(err)
	}
	if gotPeer != peer {
		t.Fatalf("peer=%q", gotPeer)
	}
	if cid != GenerateUniqueID(uid, peer) || cid != GenerateUniqueID(peer, uid) {
		t.Fatalf("cid=%q", cid)
	}

	if _, _, err := resolveChatLogSummary("", req); err == nil {
		t.Fatal("login required")
	}
	if _, _, err := resolveChatLogSummary(uid, nil); err == nil {
		t.Fatal("form required")
	}
	if _, _, err := resolveChatLogSummary(uid, &domain.ChatLogSummaryReq{PeerUserId: uid, StartTime: 1, EndTime: 2}); err == nil {
		t.Fatal("self chat")
	}
	if _, _, err := resolveChatLogSummary(uid, &domain.ChatLogSummaryReq{PeerUserId: peer, StartTime: 200, EndTime: 100, Goal: "提取待办"}); err == nil {
		t.Fatal("time window")
	}
	if _, _, err := resolveChatLogSummary(uid, &domain.ChatLogSummaryReq{PeerUserId: peer, StartTime: 100, EndTime: 200}); err == nil {
		t.Fatal("goal required")
	}
}

func TestChatRespFromChainOut(t *testing.T) {
	in := &domain.ChatResp{ChatType: domain.ChatLog, Data: domain.ChatLogSummaryData{Text: "ok", Truncated: true, TruncateNote: "cut"}}
	got, err := chatRespFromChainOut(map[string]any{langchain.OutPut: in})
	if err != nil || got.Data.(domain.ChatLogSummaryData).TruncateNote != "cut" {
		t.Fatalf("got %+v %v", got, err)
	}
	if _, err := chatRespFromChainOut(nil); err == nil {
		t.Fatal("empty")
	}
}
