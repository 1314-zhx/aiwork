/**
 * @author: dn-jinmin/dn-jinmin
 * @doc:
 */

package ws

import (
	"context"
	"github.com/gorilla/websocket"
	"aiworkc/internal/domain"
)

func (s *Ws) privateChat(ctx context.Context, conn *websocket.Conn, req *domain.Message) error {
	if err := s.chat.PrivateChat(ctx, req); err != nil {
		return err
	}

	return s.sendByUids(ctx, req)
}

func (s *Ws) groupChat(ctx context.Context, conn *websocket.Conn, req *domain.Message) error {
	if _, err := s.chat.GroupChat(ctx, req); err != nil {
		return err
	}

	return s.sendByUids(ctx, req)
}
