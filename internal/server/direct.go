package server

import (
	"chatflow/internal/hub"
	"chatflow/internal/protocol"
	"chatflow/internal/service"
	"context"
	"log/slog"
)

// ensureDirectChat находит или заводит личный чат с собеседником. О новом чате
// оба узнают из chat_added раньше, чем из него придёт первое message: в каждое
// соединение кадры уходят по порядку.
func ensureDirectChat(ctx context.Context, svc *service.Service, h *hub.Hub, userID, peerID int) (int, error) {

	chatID, created, err := svc.GetOrCreateChat(ctx, peerID, userID)
	if err != nil {
		return nullID, err
	}

	if created {
		sendChatAdded(h, userID, chatID, peerID)
		sendChatAdded(h, peerID, chatID, userID)
	}

	return chatID, nil
}

// sendChatAdded сообщает пользователю о новом личном чате; peerID — его собеседник.
func sendChatAdded(h *hub.Hub, userID, chatID, peerID int) {

	frame, err := newFrame(chatAddedType, protocol.ChatAdded{
		ChatID: chatID,
		Kind:   protocol.KindDirect,
		PeerID: peerID,
	})
	if err != nil {
		slog.Error("кадр chat_added не собран", "err", err)
		return
	}

	h.Send([]int{userID}, frame)
}
