/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gitee.com/dn-jinmin/tlog"
	"github.com/gorilla/websocket"
	"aiworkc/internal/domain"
	"aiworkc/internal/logic"
	"aiworkc/internal/model"
	"aiworkc/internal/svc"
	"aiworkc/pkg/token"
	"net/http"
	"sync"
	"time"
)

type Ws struct {
	websocket.Upgrader
	sync.RWMutex

	svc         *svc.ServiceContext
	tokenparser *token.Parse
	chat        logic.Chat

	uidToConn map[string]*websocket.Conn
	ConnToUid map[*websocket.Conn]string
}

func NewWs(svc *svc.ServiceContext) *Ws {
	tlog.Init(
		tlog.WithLoggerWriter(tlog.NewLoggerWriter()),
		tlog.WithLabel(svc.Config.Tlog.Label),
		tlog.WithMode(svc.Config.Tlog.Mode),
	)

	return &Ws{
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		svc:         svc,
		chat:        logic.NewChat(svc),
		tokenparser: token.NewTokenParse(svc.Config.Jwt.Secret),

		uidToConn: make(map[string]*websocket.Conn),
		ConnToUid: make(map[*websocket.Conn]string),
	}
}

func (s *Ws) Run() {
	http.HandleFunc("/ws", s.ServerWs)
	fmt.Println("启动websocket服务", s.svc.Config.Ws.Addr)
	http.ListenAndServe(s.svc.Config.Ws.Addr, nil)
}

func (s *Ws) ServerWs(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if e := recover(); e != nil {
			tlog.ErrorCtx(r.Context(), "serverWs", e)
		}
	}()

	// 鉴权
	uid, tok, err := s.auth(r)
	if err != nil {
		tlog.ErrorfCtx(r.Context(), "serverWs", "auth fail %v", err.Error())
		return
	}

	// 获取离线消息同步时间(从URL参数)
	lastSyncTime := int64(0)
	if lst := r.URL.Query().Get("lastSyncTime"); lst != "" {
		fmt.Sscanf(lst, "%d", &lastSyncTime)
	}

	// 请求升级
	respHeader := http.Header{
		"sec-websocket-protocol": []string{tok},
	}
	c, err := s.Upgrade(w, r, respHeader)
	if err != nil {
		tlog.ErrorfCtx(r.Context(), "serverWs", "Upgrade fail %v", err.Error())
		return
	}

	// 记录连接
	s.addConn(c, uid)

	// 连接成功后，发送离线消息和当前用户待审批记录。
	go s.sendOfflineMessages(c, uid, tok, lastSyncTime)
	go s.sendPendingApprovals(c, uid, tok)

	go s.handlerConn(c, uid, tok)
}

func (s *Ws) handlerConn(conn *websocket.Conn, uid, tok string) {
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			tlog.Errorf("serverWs", "conn.ReadMessage fail %v, uid %v", err.Error(), uid)
			s.closeConn(conn)
			return
		}

		ctx := s.context(uid, tok)

		var req domain.Message
		if err := json.Unmarshal(msg, &req); err != nil {
			tlog.ErrorfCtx(ctx, "handlerConn", "json.Unmarshal fail %v", err.Error())
			return
		}
		req.SendId = uid
		// 分发请求
		switch model.ChatType(req.ChatType) {
		case model.SingleChatType, 0: // 0 也当单聊处理
			err = s.privateChat(ctx, conn, &req)
		case model.GroupChatType:
			err = s.groupChat(ctx, conn, &req)
		}

		if err != nil {
			tlog.ErrorfCtx(ctx, "handlerConn", "message handle fail %v, msg %v", err.Error(), req)
			return
		}
	}
}
func (s *Ws) context(uid, tok string) context.Context {
	ctx := context.WithValue(context.Background(), token.Identify, uid)
	ctx = context.WithValue(ctx, token.Authorization, tok)

	return tlog.TraceStart(ctx)
}

func (s *Ws) addConn(conn *websocket.Conn, uid string) {
	s.RWMutex.Lock()
	defer s.RWMutex.Unlock()

	if c := s.uidToConn[uid]; c != nil {
		c.Close()
	}

	s.uidToConn[uid] = conn
	s.ConnToUid[conn] = uid
}

func (s *Ws) closeConn(conn *websocket.Conn) {
	s.RWMutex.Lock()
	defer s.RWMutex.Unlock()

	uid := s.ConnToUid[conn]
	if uid == "" {
		return
	}

	fmt.Printf("关闭 %s 连接\n", uid)

	delete(s.ConnToUid, conn)
	delete(s.uidToConn, uid)

	conn.Close()
}

func (s *Ws) send(ctx context.Context, conn *websocket.Conn, v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		tlog.ErrorCtx(ctx, "conn.send", err.Error())
		return err
	}

	return conn.WriteMessage(websocket.TextMessage, b)
}
// uids ...string 表示可传 0 个或多个参数
// len(uids) == 0 时 = 广播给所有在线用户
func (s *Ws) sendByUids(ctx context.Context, msg interface{}, uids ...string) error {
	s.RWMutex.RLock()
	defer s.RWMutex.RUnlock()

	if len(uids) == 0 {
		for _, conn := range s.uidToConn {
			if err := s.send(ctx, conn, msg); err != nil {
				tlog.ErrorCtx(ctx, "sendByUids.all.send", err.Error())
				return err
			}
		}
		return nil
	}

	for _, uid := range uids {
		c, ok := s.uidToConn[uid]
		if !ok {
			continue
		}
		if err := s.send(ctx, c, msg); err != nil {
			tlog.ErrorfCtx(ctx, "sendByUids.one.send err %v, uid %v", err.Error(), uid)
			return err
		}
	}
	return nil
}

func (s *Ws) PushToUser(ctx context.Context, uid string, msg interface{}) error {
	return s.sendByUids(ctx, msg, uid)
}

func (s *Ws) auth(r *http.Request) (uid string, tokenStr string, err error) {
	tok := r.Header.Get("sec-websocket-protocol")
	if tok == "" {
		return "", "", errors.New("没有登入，不存在访问权限")
	}

	claims, tokenStr, err := s.tokenparser.ParseToken(tok)
	if err != nil {
		return "", "", err
	}

	return claims[token.Identify].(string), tokenStr, nil
}

// sendOfflineMessages 连接时发送离线消息
func (s *Ws) sendPendingApprovals(conn *websocket.Conn, uid, tok string) {
	ctx := s.context(uid, tok)
	items, _, err := s.svc.ApprovalModel.List(ctx, &domain.ApprovalListReq{
		UserId: uid,
		Type:   int(model.ApprovalAudit),
	})
	if err != nil {
		tlog.ErrorfCtx(ctx, "sendPendingApprovals", "list fail: %v", err)
		return
	}

	pending := make([]*model.Approval, 0, len(items))
	for _, item := range items {
		if item.ApprovalId == uid && item.Status == model.Processed {
			pending = append(pending, item)
		}
	}

	if len(pending) == 0 {
		return
	}

	if err := s.send(ctx, conn, map[string]interface{}{
		"type":      "approval_pending_sync",
		"approvals": pending,
	}); err != nil {
		tlog.ErrorfCtx(ctx, "sendPendingApprovals", "send fail: %v", err)
	}
}

func (s *Ws) sendOfflineMessages(conn *websocket.Conn, uid, tok string, lastSyncTime int64) {
	// 等待一小段时间确保连接稳定
	time.Sleep(100 * time.Millisecond)

	ctx := s.context(uid, tok)

	// 调用 SyncAll 获取离线消息
	resp, err := s.chat.SyncAll(ctx, lastSyncTime, uid)
	if err != nil {
		tlog.ErrorfCtx(ctx, "sendOfflineMessages", "SyncAll fail: %v", err.Error())
		return
	}

	// 发送离线消息
	if len(resp.Messages) > 0 {
		msg := map[string]interface{}{
			"type":    "offline_sync",
			"messages": resp.Messages,
			"serverTime": resp.ServerTime,
		}

		if err := s.send(ctx, conn, msg); err != nil {
			tlog.ErrorfCtx(ctx, "sendOfflineMessages", "send fail: %v", err.Error())
			return
		}

		tlog.InfoCtx(ctx, "sendOfflineMessages", fmt.Sprintf("sent %d offline messages", len(resp.Messages)))
	}
}
