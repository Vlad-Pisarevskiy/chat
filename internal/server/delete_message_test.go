package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func deletePath(chatID int) string {
	return fmt.Sprintf("/chats/%d/messages/delete", chatID)
}

// deleteMessage просит удалить сообщение и возвращает статус и тело ответа.
func (e *env) deleteMessage(t *testing.T, u *user, chatID, messageID int) (int, []byte) {
	t.Helper()

	return e.postJSON(t, u, deletePath(chatID), map[string]any{"message_id": messageID})
}

// mustDelete удаляет сообщение и ждёт 204 без тела.
func (e *env) mustDelete(t *testing.T, u *user, chatID, messageID int) {
	t.Helper()

	status, body := e.deleteMessage(t, u, chatID, messageID)
	if status != http.StatusNoContent {
		t.Fatalf("удаление %d в чате %d: статус %d, ждали 204; тело %s", messageID, chatID, status, body)
	}
	if len(body) != 0 {
		t.Fatalf("у 204 есть тело: %s", body)
	}
}

// setRole выставляет роль прямо в базе: назначать роли через API пока нельзя.
func setRole(t *testing.T, chatID int, u *user, role string) {
	t.Helper()

	tag, err := testPool.Exec(context.Background(),
		`UPDATE users_chats SET role = $3 WHERE chat_id = $1 AND user_id = $2`, chatID, u.id, role)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("пользователь %d не состоит в чате %d", u.id, chatID)
	}
}

// leaveChat убирает пользователя из чата прямо в базе: так выглядят и выход,
// и исключение.
func leaveChat(t *testing.T, chatID int, u *user) {
	t.Helper()

	tag, err := testPool.Exec(context.Background(),
		`DELETE FROM users_chats WHERE chat_id = $1 AND user_id = $2`, chatID, u.id)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("пользователь %d не состоит в чате %d", u.id, chatID)
	}
}

func messageInDB(t *testing.T, messageID int) bool {
	t.Helper()

	return countRows(t, `SELECT count(*) FROM messages WHERE id = $1`, messageID) == 1
}

// historyIDs — id всех сообщений чата, которые видит пользователь.
func (e *env) historyIDs(t *testing.T, u *user, chatID int) []int {
	t.Helper()

	return e.page(t, u, chatID, historyQuery(chatID, strconv.Itoa(maxHistoryLimit), ""))
}

// requirePresent: сообщение на месте и в базе, и в истории читателя.
func (e *env) requirePresent(t *testing.T, reader *user, chatID, messageID int) {
	t.Helper()

	if !messageInDB(t, messageID) {
		t.Fatalf("сообщения %d нет в базе, а должно остаться", messageID)
	}
	if !slices.Contains(e.historyIDs(t, reader, chatID), messageID) {
		t.Fatalf("сообщения %d нет в истории чата %d, а должно остаться", messageID, chatID)
	}
}

// requireGone: сообщения нет ни в базе, ни в истории читателя.
func (e *env) requireGone(t *testing.T, reader *user, chatID, messageID int) {
	t.Helper()

	if messageInDB(t, messageID) {
		t.Fatalf("сообщение %d осталось в базе", messageID)
	}
	if slices.Contains(e.historyIDs(t, reader, chatID), messageID) {
		t.Fatalf("сообщение %d осталось в истории чата %d", messageID, chatID)
	}
}

// speaker отправляет сообщения от имени разных пользователей. Соединение
// открывается одно на пользователя в пределах теста: оно закрывается вместе с
// тестом, который его открыл.
type speaker struct {
	e     *env
	conns map[speakerKey]*wsConn
}

type speakerKey struct {
	t *testing.T
	u *user
}

func newSpeaker(e *env) *speaker {
	return &speaker{e: e, conns: make(map[speakerKey]*wsConn)}
}

func (s *speaker) say(t *testing.T, u *user, chatID int) int {
	t.Helper()

	key := speakerKey{t: t, u: u}
	c, ok := s.conns[key]
	if !ok {
		c = s.e.dial(t, u)
		s.conns[key] = c
	}

	return c.sendText(t, map[string]any{"chat_id": chatID}, "сообщение").MessageID
}

// expectDeleted проверяет, что следующий кадр — message_deleted ровно с этим
// chat_id и message_id.
func (c *wsConn) expectDeleted(t *testing.T, chatID, messageID int) {
	t.Helper()

	f := c.next(t)
	if f.Type != "message_deleted" {
		t.Fatalf("ждали message_deleted, пришёл %s %s", f.Type, f.Payload)
	}

	var got map[string]any
	if err := json.Unmarshal(f.Payload, &got); err != nil {
		t.Fatalf("payload не разобран: %s", f.Payload)
	}

	want := map[string]any{"chat_id": float64(chatID), "message_id": float64(messageID)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("message_deleted %s, ждали chat_id %d и message_id %d", f.Payload, chatID, messageID)
	}
}

func TestDeleteMessageSuccess(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	ac := e.dial(t, a)
	first := ac.sendText(t, map[string]any{"peer_id": b.id}, "первое")
	chatID := first.ChatID
	second := ac.sendText(t, map[string]any{"chat_id": chatID}, "второе").MessageID

	e.mustDelete(t, a, chatID, first.MessageID)

	// Пропало у обоих, соседнее сообщение на месте.
	for _, u := range []*user{a, b} {
		requireIDs(t, "история после удаления", e.historyIDs(t, u, chatID), []int{second})
	}
	if messageInDB(t, first.MessageID) {
		t.Fatal("удалённое сообщение осталось в базе")
	}
}

func TestDeleteMessageBadRequest(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	ack := e.dial(t, a).sendText(t, map[string]any{"peer_id": b.id}, "привет")
	path := deletePath(ack.ChatID)

	cases := []struct {
		name string
		body string
	}{
		{"не JSON", `удали`},
		{"битый JSON", `{"message_id": `},
		{"пустое тело", ``},
		{"массив", `[1]`},
		{"число вместо объекта", strconv.Itoa(ack.MessageID)},
		{"null вместо объекта", `null`},
		{"нет message_id", `{}`},
		{"message_id null", `{"message_id": null}`},
		{"message_id 0", `{"message_id": 0}`},
		{"message_id отрицательный", `{"message_id": -1}`},
		{"message_id строкой", fmt.Sprintf(`{"message_id": "%d"}`, ack.MessageID)},
		{"message_id дробный", `{"message_id": 1.5}`},
		{"message_id булев", `{"message_id": true}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := e.do(t, a, http.MethodPost, path, "application/json", strings.NewReader(tc.body))
			requireError(t, resp.StatusCode, body, http.StatusBadRequest)
		})
	}

	e.requirePresent(t, a, ack.ChatID, ack.MessageID)
}

func TestDeleteMessageNotJSON(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	ack := e.dial(t, a).sendText(t, map[string]any{"peer_id": b.id}, "привет")
	body := fmt.Sprintf(`{"message_id": %d}`, ack.MessageID)

	for _, contentType := range []string{"text/plain", "application/x-www-form-urlencoded", "multipart/form-data; boundary=x", ""} {
		t.Run("Content-Type="+contentType, func(t *testing.T) {
			resp, data := e.do(t, a, http.MethodPost, deletePath(ack.ChatID), contentType, strings.NewReader(body))
			requireError(t, resp.StatusCode, data, http.StatusUnsupportedMediaType)
		})
	}

	e.requirePresent(t, a, ack.ChatID, ack.MessageID)

	// С параметром charset это всё ещё JSON.
	resp, data := e.do(t, a, http.MethodPost, deletePath(ack.ChatID), "application/json; charset=utf-8", strings.NewReader(body))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("application/json; charset=utf-8: %d %s", resp.StatusCode, data)
	}
}

func TestDeleteMessageUnauthorized(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	ack := e.dial(t, a).sendText(t, map[string]any{"peer_id": b.id}, "привет")

	status, body := e.deleteMessage(t, nil, ack.ChatID, ack.MessageID)
	requireError(t, status, body, http.StatusUnauthorized)

	bogus := &user{cookie: &http.Cookie{Name: "token", Value: "нет-такого-токена"}}
	status, body = e.deleteMessage(t, bogus, ack.ChatID, ack.MessageID)
	requireError(t, status, body, http.StatusUnauthorized)

	// Без сессии 401 раньше проверки типа тела.
	resp, data := e.do(t, nil, http.MethodPost, deletePath(ack.ChatID), "text/plain", strings.NewReader("x"))
	requireError(t, resp.StatusCode, data, http.StatusUnauthorized)

	e.requirePresent(t, a, ack.ChatID, ack.MessageID)
}

// GET ничего не удаляет: меняющие ручки его не принимают.
func TestDeleteMessageNotByGet(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	ack := e.dial(t, a).sendText(t, map[string]any{"peer_id": b.id}, "привет")

	resp, body := e.get(t, a, deletePath(ack.ChatID)+"?message_id="+strconv.Itoa(ack.MessageID))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		t.Fatalf("GET на удаление ответил %d %s", resp.StatusCode, body)
	}

	e.requirePresent(t, a, ack.ChatID, ack.MessageID)
}

func TestDeleteMessageForbidden(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b, stranger := e.newUser(t), e.newUser(t), e.newUser(t)
	ack := e.dial(t, a).sendText(t, map[string]any{"peer_id": b.id}, "привет")

	t.Run("не участник", func(t *testing.T) {
		status, body := e.deleteMessage(t, stranger, ack.ChatID, ack.MessageID)
		requireError(t, status, body, http.StatusForbidden)
	})

	t.Run("чужое сообщение", func(t *testing.T) {
		status, body := e.deleteMessage(t, b, ack.ChatID, ack.MessageID)
		requireError(t, status, body, http.StatusForbidden)
	})

	e.requirePresent(t, a, ack.ChatID, ack.MessageID)
}

func TestDeleteMessageNotFound(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b, c := e.newUser(t), e.newUser(t), e.newUser(t)
	s := newSpeaker(e)

	direct := e.directChat(t, a, b)
	group := e.createGroup(t, a, b, c)
	mine := s.say(t, a, direct)
	elsewhere := s.say(t, a, group)

	cases := []struct {
		name    string
		path    string
		message int
	}{
		{"чата нет", deletePath(missingID), mine},
		{"чат не число", "/chats/abc/messages/delete", mine},
		{"сообщения нет", deletePath(direct), missingID},
		// Своё сообщение, но из другого чата: удаляется только то, что лежит в
		// чате из пути.
		{"сообщение из другого чата", deletePath(direct), elsewhere},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := e.postJSON(t, a, tc.path, map[string]any{"message_id": tc.message})
			requireError(t, status, body, http.StatusNotFound)
		})
	}

	e.requirePresent(t, a, direct, mine)
	e.requirePresent(t, a, group, elsewhere)
}

// Проверки идут по порядку: чат есть → участник → сообщение в этом чате →
// права.
func TestDeleteMessageCheckOrder(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b, c, stranger := e.newUser(t), e.newUser(t), e.newUser(t), e.newUser(t)
	s := newSpeaker(e)

	direct := e.directChat(t, a, b)
	group := e.createGroup(t, a, b, c)
	strangers := e.createGroup(t, stranger, c)

	inDirect := s.say(t, a, direct)
	inGroup := s.say(t, a, group)
	ownElsewhere := s.say(t, stranger, strangers)

	cases := []struct {
		name    string
		u       *user
		chatID  int
		message int
		want    int
	}{
		{"чата нет, сообщения нет", stranger, missingID, missingID, http.StatusNotFound},
		{"чата нет, сообщение есть", a, missingID, inDirect, http.StatusNotFound},
		{"не участник, сообщения нет", stranger, direct, missingID, http.StatusForbidden},
		{"не участник, сообщение из другого чата", stranger, direct, inGroup, http.StatusForbidden},
		{"не участник, своё сообщение из другого чата", stranger, direct, ownElsewhere, http.StatusForbidden},
		// В группе b не может удалить сообщение a, но сообщения нет в чате из
		// пути — это проверяется раньше прав.
		{"чужое сообщение из другого чата", b, direct, inGroup, http.StatusNotFound},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := e.deleteMessage(t, tc.u, tc.chatID, tc.message)
			requireError(t, status, body, tc.want)
		})
	}

	e.requirePresent(t, a, direct, inDirect)
	e.requirePresent(t, a, group, inGroup)
	e.requirePresent(t, stranger, strangers, ownElsewhere)
}

// rightsCase — одна клетка матрицы прав: author пишет сообщение, actor его
// удаляет.
type rightsCase struct {
	name    string
	author  *user
	actor   *user
	allowed bool
}

// runRights прогоняет клетки по одной. reader — тот, кто читает историю для
// проверки: он состоит в чате до конца.
func runRights(t *testing.T, e *env, s *speaker, chatID int, reader *user, cases []rightsCase) {
	t.Helper()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			messageID := s.say(t, tc.author, chatID)

			if tc.allowed {
				e.mustDelete(t, tc.actor, chatID, messageID)
				e.requireGone(t, reader, chatID, messageID)
				return
			}

			status, body := e.deleteMessage(t, tc.actor, chatID, messageID)
			requireError(t, status, body, http.StatusForbidden)
			e.requirePresent(t, reader, chatID, messageID)
		})
	}
}

func TestDeleteMessageRightsDirect(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	s := newSpeaker(e)
	chatID := e.directChat(t, a, b)

	runRights(t, e, s, chatID, a, []rightsCase{
		{name: "своё", author: a, actor: a, allowed: true},
		{name: "своё у собеседника", author: b, actor: b, allowed: true},
		{name: "чужое", author: b, actor: a},
		{name: "чужое у собеседника", author: a, actor: b},
	})
}

func TestDeleteMessageRightsGroup(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	owner, admin, otherAdmin := e.newUser(t), e.newUser(t), e.newUser(t)
	member, otherMember := e.newUser(t), e.newUser(t)
	s := newSpeaker(e)

	chatID := e.createGroup(t, owner, admin, otherAdmin, member, otherMember)
	setRole(t, chatID, owner, "owner")
	setRole(t, chatID, admin, "admin")
	setRole(t, chatID, otherAdmin, "admin")

	runRights(t, e, s, chatID, owner, []rightsCase{
		{name: "member: своё", author: member, actor: member, allowed: true},
		{name: "member: другого member", author: otherMember, actor: member},
		{name: "member: admin", author: admin, actor: member},
		{name: "member: owner", author: owner, actor: member},

		{name: "admin: своё", author: admin, actor: admin, allowed: true},
		{name: "admin: member", author: member, actor: admin, allowed: true},
		{name: "admin: другого admin", author: otherAdmin, actor: admin},
		{name: "admin: owner", author: owner, actor: admin},

		{name: "owner: своё", author: owner, actor: owner, allowed: true},
		{name: "owner: member", author: member, actor: owner, allowed: true},
		{name: "owner: admin", author: admin, actor: owner, allowed: true},
	})
}

// Роли берутся на момент удаления — и у удаляющего, и у автора.
func TestDeleteMessageRightsGroupRoleChanges(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	t.Run("автор вышел", func(t *testing.T) {
		owner, admin, member, author := e.newUser(t), e.newUser(t), e.newUser(t), e.newUser(t)
		s := newSpeaker(e)
		chatID := e.createGroup(t, owner, admin, member, author)
		setRole(t, chatID, owner, "owner")
		setRole(t, chatID, admin, "admin")

		// Пишет три сообщения, пока состоит в группе, и уходит.
		first, second, third := s.say(t, author, chatID), s.say(t, author, chatID), s.say(t, author, chatID)
		leaveChat(t, chatID, author)

		e.mustDelete(t, admin, chatID, first)
		e.requireGone(t, owner, chatID, first)

		e.mustDelete(t, owner, chatID, second)
		e.requireGone(t, owner, chatID, second)

		// Сам автор уже не участник.
		status, body := e.deleteMessage(t, author, chatID, third)
		requireError(t, status, body, http.StatusForbidden)
		e.requirePresent(t, owner, chatID, third)

		// Рядовой участник чужое сообщение ушедшего не удаляет.
		status, body = e.deleteMessage(t, member, chatID, third)
		requireError(t, status, body, http.StatusForbidden)
		e.requirePresent(t, owner, chatID, third)
	})

	t.Run("ушедший admin считается member", func(t *testing.T) {
		owner, admin, former := e.newUser(t), e.newUser(t), e.newUser(t)
		s := newSpeaker(e)
		chatID := e.createGroup(t, owner, admin, former)
		setRole(t, chatID, owner, "owner")
		setRole(t, chatID, admin, "admin")
		setRole(t, chatID, former, "admin")

		messageID := s.say(t, former, chatID)
		leaveChat(t, chatID, former)

		e.mustDelete(t, admin, chatID, messageID)
		e.requireGone(t, owner, chatID, messageID)
	})

	owner, admin, demoted, promoted, member := e.newUser(t), e.newUser(t), e.newUser(t), e.newUser(t), e.newUser(t)
	s := newSpeaker(e)
	chatID := e.createGroup(t, owner, admin, demoted, promoted, member)
	setRole(t, chatID, owner, "owner")
	setRole(t, chatID, admin, "admin")

	t.Run("автора-admin понизили после отправки", func(t *testing.T) {
		setRole(t, chatID, demoted, "admin")
		messageID := s.say(t, demoted, chatID)

		// Пока он admin, другой admin его сообщение не удаляет.
		status, body := e.deleteMessage(t, admin, chatID, messageID)
		requireError(t, status, body, http.StatusForbidden)

		setRole(t, chatID, demoted, "member")
		e.mustDelete(t, admin, chatID, messageID)
		e.requireGone(t, owner, chatID, messageID)
	})

	t.Run("автора повысили до admin после отправки", func(t *testing.T) {
		messageID := s.say(t, promoted, chatID)
		setRole(t, chatID, promoted, "admin")

		status, body := e.deleteMessage(t, admin, chatID, messageID)
		requireError(t, status, body, http.StatusForbidden)
		e.requirePresent(t, owner, chatID, messageID)
	})

	t.Run("удаляющего admin понизили", func(t *testing.T) {
		messageID := s.say(t, member, chatID)
		setRole(t, chatID, admin, "member")
		t.Cleanup(func() { setRole(t, chatID, admin, "admin") })

		status, body := e.deleteMessage(t, admin, chatID, messageID)
		requireError(t, status, body, http.StatusForbidden)
		e.requirePresent(t, owner, chatID, messageID)
	})
}

func TestDeleteMessageRightsChannel(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	owner, admin, otherAdmin, member, former := e.newUser(t), e.newUser(t), e.newUser(t), e.newUser(t), e.newUser(t)
	s := newSpeaker(e)

	chatID := e.createChannel(t, owner)
	for _, u := range []*user{admin, otherAdmin, member, former} {
		e.joinChannel(t, u, chatID)
	}
	setRole(t, chatID, admin, "admin")
	setRole(t, chatID, otherAdmin, "admin")

	// Пост бывшего админа: писал, пока был admin, затем его понизили до member.
	formerPost := func(t *testing.T) int {
		t.Helper()

		setRole(t, chatID, former, "admin")
		messageID := s.say(t, former, chatID)
		setRole(t, chatID, former, "member")

		return messageID
	}

	runRights(t, e, s, chatID, owner, []rightsCase{
		{name: "member: пост owner", author: owner, actor: member},
		{name: "member: пост admin", author: admin, actor: member},

		{name: "admin: свой пост", author: admin, actor: admin, allowed: true},
		{name: "admin: пост другого admin", author: otherAdmin, actor: admin, allowed: true},
		{name: "admin: пост owner", author: owner, actor: admin},

		{name: "owner: свой пост", author: owner, actor: owner, allowed: true},
		{name: "owner: пост admin", author: admin, actor: owner, allowed: true},
	})

	cases := []struct {
		name    string
		actor   *user
		allowed bool
	}{
		// Понижённый до member свой прежний пост уже не удаляет.
		{"бывший admin: свой прежний пост", former, false},
		{"member: пост бывшего admin", member, false},
		{"admin: пост бывшего admin", admin, true},
		{"owner: пост бывшего admin", owner, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			messageID := formerPost(t)

			if tc.allowed {
				e.mustDelete(t, tc.actor, chatID, messageID)
				e.requireGone(t, owner, chatID, messageID)
				return
			}

			status, body := e.deleteMessage(t, tc.actor, chatID, messageID)
			requireError(t, status, body, http.StatusForbidden)
			e.requirePresent(t, owner, chatID, messageID)
		})
	}
}

func TestDeleteMessageTwice(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	owner, author := e.newUser(t), e.newUser(t)
	s := newSpeaker(e)
	chatID := e.createGroup(t, owner, author)
	setRole(t, chatID, owner, "owner")

	messageID := s.say(t, author, chatID)
	e.mustDelete(t, author, chatID, messageID)

	// Повтор тем же пользователем и другим, у которого права есть.
	for _, u := range []*user{author, owner} {
		status, body := e.deleteMessage(t, u, chatID, messageID)
		requireError(t, status, body, http.StatusNotFound)
	}
}

func TestDeleteMessageWithImage(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	imageRows := func(t *testing.T, imageID int) int {
		t.Helper()
		return countRows(t, `SELECT count(*) FROM images WHERE id = $1`, imageID)
	}

	t.Run("автор в личке", func(t *testing.T) {
		a, b := e.newUser(t), e.newUser(t)
		gone := e.sendImage(t, a, toPeer(b, newUUID(t), "закат"), pngFile(t))
		kept := e.sendImage(t, a, toChat(gone.ChatID, newUUID(t), ""), jpegFile(t))

		e.mustDelete(t, a, gone.ChatID, gone.MessageID)
		e.requireGone(t, b, gone.ChatID, gone.MessageID)

		for _, u := range []*user{a, b} {
			resp, body := e.get(t, u, fmt.Sprintf("/images/%d", gone.ImageID))
			requireError(t, resp.StatusCode, body, http.StatusNotFound)

			// Соседняя картинка не задета.
			resp, body = e.get(t, u, fmt.Sprintf("/images/%d", kept.ImageID))
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("соседняя картинка: %d %s", resp.StatusCode, body)
			}
		}

		if n := imageRows(t, gone.ImageID); n != 0 {
			t.Fatalf("строк картинки после удаления %d, ждали 0", n)
		}
		if n := imageRows(t, kept.ImageID); n != 1 {
			t.Fatalf("строк соседней картинки %d, ждали 1", n)
		}
	})

	t.Run("admin канала удаляет чужой пост", func(t *testing.T) {
		owner, admin, otherAdmin := e.newUser(t), e.newUser(t), e.newUser(t)
		chatID := e.createChannel(t, owner)
		e.joinChannel(t, admin, chatID)
		e.joinChannel(t, otherAdmin, chatID)
		setRole(t, chatID, admin, "admin")
		setRole(t, chatID, otherAdmin, "admin")

		post := e.sendImage(t, otherAdmin, toChat(chatID, newUUID(t), "пост"), pngFile(t))

		e.mustDelete(t, admin, chatID, post.MessageID)
		e.requireGone(t, owner, chatID, post.MessageID)

		resp, body := e.get(t, owner, fmt.Sprintf("/images/%d", post.ImageID))
		requireError(t, resp.StatusCode, body, http.StatusNotFound)

		if n := imageRows(t, post.ImageID); n != 0 {
			t.Fatalf("строк картинки после удаления %d, ждали 0", n)
		}
	})

	t.Run("отказ картинку не трогает", func(t *testing.T) {
		a, b := e.newUser(t), e.newUser(t)
		sent := e.sendImage(t, a, toPeer(b, newUUID(t), ""), pngFile(t))

		status, body := e.deleteMessage(t, b, sent.ChatID, sent.MessageID)
		requireError(t, status, body, http.StatusForbidden)

		resp, body := e.get(t, b, fmt.Sprintf("/images/%d", sent.ImageID))
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("картинка после отказа: %d %s", resp.StatusCode, body)
		}
	})
}

func TestDeleteMessageBroadcast(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	t.Run("личка", func(t *testing.T) {
		a, b, stranger := e.newUser(t), e.newUser(t), e.newUser(t)
		s := newSpeaker(e)
		chatID := e.directChat(t, a, b)
		messageID := s.say(t, a, chatID)

		// Соединения открываются после отправки: первым кадром должен прийти
		// message_deleted.
		a1, a2, b1, b2, sc := e.dial(t, a), e.dial(t, a), e.dial(t, b), e.dial(t, b), e.dial(t, stranger)

		e.mustDelete(t, a, chatID, messageID)

		for _, c := range []*wsConn{a1, a2, b1, b2} {
			c.expectDeleted(t, chatID, messageID)
		}

		barrier := e.createGroup(t, stranger, a)
		sc.expectBarrier(t, barrier)
		// После кадра удаления у участников больше ничего не было.
		a1.expectBarrier(t, barrier)
	})

	t.Run("группа, удаляет не автор", func(t *testing.T) {
		owner, author, member, left, stranger := e.newUser(t), e.newUser(t), e.newUser(t), e.newUser(t), e.newUser(t)
		s := newSpeaker(e)
		chatID := e.createGroup(t, owner, author, member, left)
		setRole(t, chatID, owner, "owner")
		messageID := s.say(t, author, chatID)
		leaveChat(t, chatID, left)

		o1, o2 := e.dial(t, owner), e.dial(t, owner)
		ac, mc, lc, sc := e.dial(t, author), e.dial(t, member), e.dial(t, left), e.dial(t, stranger)

		e.mustDelete(t, owner, chatID, messageID)

		for _, c := range []*wsConn{o1, o2, ac, mc} {
			c.expectDeleted(t, chatID, messageID)
		}

		// Ушедший из группы и посторонний кадр не получают.
		barrier := e.createGroup(t, stranger, left)
		sc.expectBarrier(t, barrier)
		lc.expectBarrier(t, barrier)
	})

	t.Run("канал", func(t *testing.T) {
		owner, admin, subscriber, stranger := e.newUser(t), e.newUser(t), e.newUser(t), e.newUser(t)
		s := newSpeaker(e)
		chatID := e.createChannel(t, owner)
		e.joinChannel(t, admin, chatID)
		e.joinChannel(t, subscriber, chatID)
		setRole(t, chatID, admin, "admin")
		messageID := s.say(t, admin, chatID)

		oc, ac, subc, sc := e.dial(t, owner), e.dial(t, admin), e.dial(t, subscriber), e.dial(t, stranger)

		e.mustDelete(t, admin, chatID, messageID)

		for _, c := range []*wsConn{oc, ac, subc} {
			c.expectDeleted(t, chatID, messageID)
		}

		barrier := e.createGroup(t, stranger, owner)
		sc.expectBarrier(t, barrier)
	})

	t.Run("отказ ничего не рассылает", func(t *testing.T) {
		a, b, stranger := e.newUser(t), e.newUser(t), e.newUser(t)
		s := newSpeaker(e)
		chatID := e.directChat(t, a, b)
		messageID := s.say(t, a, chatID)

		ac, bc := e.dial(t, a), e.dial(t, b)

		cases := []struct {
			u       *user
			message int
			want    int
		}{
			{b, messageID, http.StatusForbidden},
			{stranger, messageID, http.StatusForbidden},
			{a, missingID, http.StatusNotFound},
		}
		for _, tc := range cases {
			status, body := e.deleteMessage(t, tc.u, chatID, tc.message)
			requireError(t, status, body, tc.want)
		}

		barrier := e.createGroup(t, a, b)
		ac.expectBarrier(t, barrier)
		bc.expectBarrier(t, barrier)
	})
}

// Одно сообщение удаляют одновременно: ровно один 204, остальные 404, и кадр
// уходит один раз.
func TestDeleteMessageConcurrent(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	const (
		rounds   = 5
		requests = 6
	)

	owner, author := e.newUser(t), e.newUser(t)
	s := newSpeaker(e)
	chatID := e.createGroup(t, owner, author)
	setRole(t, chatID, owner, "owner")

	messages := make([]int, rounds)
	for i := range messages {
		messages[i] = s.say(t, author, chatID)
	}

	watcher := e.dial(t, owner)

	for _, messageID := range messages {
		// Удаляют автор и владелец вперемешку: оба вправе.
		actors := make([]*user, requests)
		for i := range actors {
			actors[i] = []*user{author, owner}[i%2]
		}

		statuses := concurrentDeletes(t, e, actors, chatID, messageID)

		var deleted, notFound int
		for _, r := range statuses {
			switch r.status {
			case http.StatusNoContent:
				deleted++
			case http.StatusNotFound:
				notFound++
				requireError(t, r.status, r.body, http.StatusNotFound)
			default:
				t.Fatalf("сообщение %d: статус %d %s", messageID, r.status, r.body)
			}
		}
		if deleted != 1 || notFound != requests-1 {
			t.Fatalf("сообщение %d: 204 — %d, 404 — %d; ждали 1 и %d", messageID, deleted, notFound, requests-1)
		}

		watcher.expectDeleted(t, chatID, messageID)
		e.requireGone(t, owner, chatID, messageID)
	}

	barrier := e.createGroup(t, owner, author)
	watcher.expectBarrier(t, barrier)
}

type deleteResult struct {
	status int
	body   []byte
}

// concurrentDeletes отправляет запросы на удаление одновременно, по одному от
// каждого actors[i].
func concurrentDeletes(t *testing.T, e *env, actors []*user, chatID, messageID int) []deleteResult {
	t.Helper()

	raw, err := json.Marshal(map[string]any{"message_id": messageID})
	if err != nil {
		t.Fatal(err)
	}

	results := make([]deleteResult, len(actors))
	errs := make([]error, len(actors))
	start := make(chan struct{})
	var wg sync.WaitGroup

	for i, u := range actors {
		wg.Go(func() {
			<-start

			req, err := http.NewRequest(http.MethodPost, e.srv.URL+deletePath(chatID), bytes.NewReader(raw))
			if err != nil {
				errs[i] = err
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(&http.Cookie{Name: u.cookie.Name, Value: u.cookie.Value})

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				errs[i] = err
				return
			}
			defer func() {
				_ = resp.Body.Close()
			}()

			results[i].body, errs[i] = io.ReadAll(resp.Body)
			results[i].status = resp.StatusCode
		})
	}

	close(start)
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	return results
}

// Пагинация по before не ломается от пропусков в нумерации.
func TestDeleteMessagePaging(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	chatID := e.createGroup(t, a, b)
	ids := e.dial(t, a).sendMany(t, chatID, 12)

	removed := []int{ids[2], ids[3], ids[7], ids[11]}
	for _, id := range removed {
		e.mustDelete(t, a, chatID, id)
	}

	var rest []int
	for _, id := range ids {
		if !slices.Contains(removed, id) {
			rest = append(rest, id)
		}
	}

	for _, u := range []*user{a, b} {
		requireIDs(t, "вся история", e.historyIDs(t, u, chatID), rest)

		// Свежая пачка — последние оставшиеся, а не меньше из-за удалённого
		// самого нового.
		requireIDs(t, "свежая пачка", e.page(t, u, chatID, historyQuery(chatID, "3", "")), lastN(rest, 3))

		// Курсор на удалённом id работает как обычный.
		for _, cursor := range removed {
			got := e.page(t, u, chatID, historyQuery(chatID, "3", strconv.Itoa(cursor)))
			requireIDs(t, "курсор на удалённом "+strconv.Itoa(cursor), got, beforeCursor(rest, cursor, 3))
		}

		// Листание до начала пачками по 3 собирает ровно оставшиеся.
		var paged []int
		cursor := ""
		for range len(ids) {
			got := e.page(t, u, chatID, historyQuery(chatID, "3", cursor))
			paged = slices.Concat(got, paged)
			if len(got) < 3 {
				break
			}
			cursor = strconv.Itoa(got[0])
		}
		requireIDs(t, "склейка пачек", paged, rest)
	}
}
