package server

import (
	errors1 "chatflow/internal/app-errors"
	"chatflow/internal/hub"
	"chatflow/internal/protocol"
	"chatflow/internal/service"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"time"

	"github.com/gorilla/websocket"
)

type session struct {
	userID  int
	conn    *websocket.Conn
	msgChan chan protocol.Data
	hub     *hub.Hub
	service *service.Service
	ctx     context.Context
	cancel  context.CancelFunc
}

func newSession(userID int, conn *websocket.Conn, hub2 *hub.Hub, service2 *service.Service) *session {

	var sess = &session{
		userID:  userID,
		conn:    conn,
		msgChan: make(chan protocol.Data, msgBufferSize),
		hub:     hub2,
		service: service2,
	}

	sess.ctx, sess.cancel = context.WithCancel(context.Background())

	return sess
}

func (s *session) handle() {

	userConn, status := s.hub.Add(s.userID, s.conn, s.msgChan, s.ctx.Done())
	defer func() {
		status = s.hub.RemoveConn(userConn)
		if status == hub.StatusOffline {
			s.sendPresence(false)
		}
	}()

	if status == hub.StatusOnline {
		s.sendPresence(true)
	}

	defer s.cancel()
	defer func(conn *websocket.Conn) {
		_ = conn.Close()
	}(s.conn)

	s.conn.SetPongHandler(func(string) error { return s.conn.SetReadDeadline(time.Now().Add(readDeadline)) })
	if err := s.conn.SetReadDeadline(time.Now().Add(readDeadline)); err != nil {
		slog.Warn("срок чтения не выставлен", "user", s.userID, "err", err)
		return
	}

	s.conn.SetReadLimit(readLimit)

	go s.writer()

	s.reader()
}

func (s *session) sendPresence(isOnline bool) {

	frame, err := newFrame(presenceType, protocol.Presence{
		UserID: s.userID,
		Online: isOnline,
	})
	if err != nil {
		slog.Error("кадр presence не собран", "err", err)
		return
	}

	s.hub.Send(s.hub.OnlineUsers(), frame)
}

func (s *session) reader() {

	for {
		if err := s.readMessage(); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Info("соединение оборвалось", "user", s.userID, "err", err)
			}
			return
		}
	}
}

func (s *session) readMessage() error {

	_, message, err := s.conn.ReadMessage()
	if err != nil {
		return err
	}

	s.dispatch(message)

	return s.conn.SetReadDeadline(time.Now().Add(readDeadline))
}

// dispatch разбирает кадр от клиента. На всё, что не удалось обработать,
// уходит error: молча выброшенный send оставил бы пузырь в «отправляется».
func (s *session) dispatch(message []byte) {

	var data protocol.Data
	if err := json.Unmarshal(message, &data); err != nil {
		s.sendError(emptyClientMsgID, errors1.ErrBadFrame)
		return
	}

	switch data.Type {
	case sendType:
		s.handleSend(data.Payload)
	default:
		s.sendError(emptyClientMsgID, errors1.ErrUnknownFrame)
	}
}

// handleSend отвечает на send ровно одним кадром: ack или error.
func (s *session) handleSend(payload json.RawMessage) {

	var send protocol.Send
	if err := json.Unmarshal(payload, &send); err != nil {
		s.sendError(clientMsgIDOf(payload), errors1.ErrBadFrame)
		return
	}

	// В ответах id ровно в том виде, в каком его прислали: по нему фронт ищет пузырь.
	clientMsgID := send.ClientMsgID

	send, err := s.service.PrepareSend(send, s.userID)
	if err != nil {
		s.sendError(clientMsgID, err)
		return
	}

	if send.PeerID != nullID {
		if send.ChatID, err = s.directChat(send.PeerID); err != nil {
			s.sendError(clientMsgID, err)
			return
		}
	}

	sent, err := s.service.SendMessage(s.ctx, send, s.userID)
	if err != nil {
		s.sendError(clientMsgID, err)
		return
	}

	s.enqueue(ackType, protocol.Ack{
		ClientMsgID: clientMsgID,
		MessageID:   sent.Message.Id,
		ChatID:      sent.Message.ChatID,
		Time:        sent.Message.Time,
	})

	// Повтор: сообщение разослали, когда сохраняли его в первый раз.
	if sent.Duplicate {
		return
	}

	frame, err := newFrame(messageType, sent.Message)
	if err != nil {
		slog.Error("кадр message не собран", "err", err)
		return
	}

	s.hub.Send(sent.Recipients, frame)
}

// directChat находит или заводит личный чат с собеседником. О новом чате оба
// узнают из chat_added раньше, чем из него придёт первое message: в каждое
// соединение кадры уходят по порядку.
func (s *session) directChat(peerID int) (int, error) {

	chatID, created, err := s.service.GetOrCreateChat(s.ctx, peerID, s.userID)
	if err != nil {
		return nullID, err
	}

	if created {
		s.sendChatAdded(s.userID, chatID, peerID)
		s.sendChatAdded(peerID, chatID, s.userID)
	}

	return chatID, nil
}

// sendChatAdded сообщает пользователю о новом личном чате; peerID — его собеседник.
func (s *session) sendChatAdded(userID, chatID, peerID int) {

	frame, err := newFrame(chatAddedType, protocol.ChatAdded{
		ChatID: chatID,
		Kind:   protocol.KindDirect,
		PeerID: peerID,
	})
	if err != nil {
		slog.Error("кадр chat_added не собран", "err", err)
		return
	}

	s.hub.Send([]int{userID}, frame)
}

func (s *session) sendError(clientMsgID string, err error) {

	f, known := classify(err)
	if !known {
		slog.Error("ошибка обработки кадра", "user", s.userID, "err", err)
	}

	s.enqueue(errorType, protocol.Error{
		Code:        f.code,
		Description: f.text,
		ClientMsgID: clientMsgID,
	})
}

// enqueue кладёт кадр в очередь своего соединения. Ждать здесь можно: очередь
// разгребает writer этого же соединения, чужих оно не задерживает.
func (s *session) enqueue(frameType string, payload any) {

	frame, err := newFrame(frameType, payload)
	if err != nil {
		slog.Error("кадр не собран", "type", frameType, "err", err)
		return
	}

	select {
	case s.msgChan <- *frame:
	case <-s.ctx.Done():
	}
}

// clientMsgIDOf достаёт client_msg_id из send, который не разобрался целиком:
// без него фронт не узнает, какое сообщение не ушло.
func clientMsgIDOf(payload json.RawMessage) string {

	var probe struct {
		ClientMsgID string `json:"client_msg_id"`
	}
	_ = json.Unmarshal(payload, &probe)

	return probe.ClientMsgID
}

func (s *session) writer() {

	ticker := time.NewTicker(tickerTiming)

	defer ticker.Stop()

	for {
		select {
		case msg := <-s.msgChan:
			if err := s.write(msg); err != nil {
				s.drop(err)
				return
			}

		case <-ticker.C:
			if err := s.ping(); err != nil {
				s.drop(err)
				return
			}

		case <-s.ctx.Done():
			return
		}
	}
}

// drop закрывает соединение, в которое не удалось писать. Читающая горутина
// выйдет из ReadMessage, и сессия завершится обычным путём.
func (s *session) drop(err error) {

	// Соединение уже закрыто читающей стороной: кадр взяли из очереди в момент
	// выхода. Это обычное завершение, а не сбой.
	if !errors.Is(err, net.ErrClosed) {
		slog.Info("запись в соединение не удалась", "user", s.userID, "err", err)
	}

	_ = s.conn.Close()
}

func (s *session) write(msg protocol.Data) error {

	if err := s.conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
		return err
	}

	message, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return s.conn.WriteMessage(websocket.TextMessage, message)
}

func (s *session) ping() error {

	if err := s.conn.SetWriteDeadline(time.Now().Add(writeDeadline)); err != nil {
		return err
	}

	return s.conn.WriteMessage(websocket.PingMessage, nil)
}
