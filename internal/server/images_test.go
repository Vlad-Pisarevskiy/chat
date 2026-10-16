package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

const (
	maxImageSize = 5 << 20

	cacheControl = "private, max-age=31536000, immutable"

	// Такого чата и такого пользователя в тестовой базе нет.
	missingID = 2_000_000_000
)

// filePart — часть file формы. name и contentType присылает клиент, сервер
// им не верит.
type filePart struct {
	name        string
	contentType string
	data        []byte
}

type imageResponse struct {
	MessageID int    `json:"message_id"`
	ChatID    int    `json:"chat_id"`
	ImageID   int    `json:"image_id"`
	Time      string `json:"time"`
}

// messagePayload — payload кадра message и элемент истории. Указатели — чтобы
// отличить отсутствующее поле от нулевого.
type messagePayload struct {
	MessageID int     `json:"message_id"`
	ChatID    int     `json:"chat_id"`
	SenderID  int     `json:"sender_id"`
	Body      *string `json:"body"`
	ImageID   *int    `json:"image_id"`
	Time      string  `json:"time"`
}

func newUUID(t *testing.T) string {
	t.Helper()

	return uuid.NewString()
}

func testImage() image.Image {

	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := range 8 {
		for y := range 8 {
			img.Set(x, y, color.RGBA{R: uint8(x * 30), G: uint8(y * 30), B: 200, A: 255})
		}
	}

	return img
}

func pngBytes(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := png.Encode(&buf, testImage()); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

func jpegBytes(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, testImage(), nil); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

func gifBytes(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := gif.Encode(&buf, testImage(), nil); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

// jpegOfSize — JPEG ровно size байт: настоящая картинка, добитая нулями.
func jpegOfSize(t *testing.T, size int) []byte {
	t.Helper()

	data := make([]byte, size)
	copy(data, jpegBytes(t))

	return data
}

func pngFile(t *testing.T) *filePart {
	t.Helper()

	return &filePart{name: "photo.png", contentType: "image/png", data: pngBytes(t)}
}

func jpegFile(t *testing.T) *filePart {
	t.Helper()

	return &filePart{name: "photo.jpg", contentType: "image/jpeg", data: jpegBytes(t)}
}

// imageForm собирает multipart-тело. file == nil — без файла.
func imageForm(t *testing.T, fields map[string]string, file *filePart) ([]byte, string) {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	for name, value := range fields {
		if err := w.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}

	if file != nil {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, file.name))
		header.Set("Content-Type", file.contentType)

		part, err := w.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = part.Write(file.data); err != nil {
			t.Fatal(err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes(), w.FormDataContentType()
}

func (e *env) postImage(t *testing.T, u *user, fields map[string]string, file *filePart) (int, []byte) {
	t.Helper()

	body, contentType := imageForm(t, fields, file)
	resp, data := e.do(t, u, http.MethodPost, "/chats/images", contentType, bytes.NewReader(body))

	return resp.StatusCode, data
}

func toChat(chatID int, clientMsgID, body string) map[string]string {
	return map[string]string{"chat_id": strconv.Itoa(chatID), "client_msg_id": clientMsgID, "body": body}
}

func toPeer(peer *user, clientMsgID, body string) map[string]string {
	return map[string]string{"peer_id": strconv.Itoa(peer.id), "client_msg_id": clientMsgID, "body": body}
}

// sendImage отправляет картинку и ждёт 201.
func (e *env) sendImage(t *testing.T, u *user, fields map[string]string, file *filePart) imageResponse {
	t.Helper()

	status, body := e.postImage(t, u, fields, file)
	if status != http.StatusCreated {
		t.Fatalf("картинка не отправлена: %d %s", status, body)
	}

	return decodeImageResponse(t, body)
}

func decodeImageResponse(t *testing.T, body []byte) imageResponse {
	t.Helper()

	var resp imageResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("ответ не разобран: %v; %s", err, body)
	}
	if resp.MessageID == 0 || resp.ChatID == 0 || resp.ImageID == 0 {
		t.Fatalf("в ответе нет message_id, chat_id или image_id: %s", body)
	}
	requireUTC(t, resp.Time)

	return resp
}

func requireUTC(t *testing.T, value string) {
	t.Helper()

	if _, err := time.Parse(time.RFC3339, value); err != nil || !strings.HasSuffix(value, "Z") {
		t.Fatalf("время не RFC 3339 в UTC: %q", value)
	}
}

// expectImageMessage проверяет, что следующий кадр — message с этой картинкой.
func (c *wsConn) expectImageMessage(t *testing.T, sent imageResponse, sender *user, body string) {
	t.Helper()

	f := c.next(t)
	if f.Type != "message" {
		t.Fatalf("ждали message, пришёл %s %s", f.Type, f.Payload)
	}

	var msg messagePayload
	if err := json.Unmarshal(f.Payload, &msg); err != nil {
		t.Fatal(err)
	}

	if msg.MessageID != sent.MessageID || msg.ChatID != sent.ChatID || msg.SenderID != sender.id {
		t.Fatalf("не то сообщение: %s, ждали %+v от %d", f.Payload, sent, sender.id)
	}
	if msg.ImageID == nil || *msg.ImageID != sent.ImageID {
		t.Fatalf("image_id в кадре %s, ждали %d", f.Payload, sent.ImageID)
	}
	if msg.Body == nil || *msg.Body != body {
		t.Fatalf("body в кадре %s, ждали %q", f.Payload, body)
	}
	requireUTC(t, msg.Time)
}

// expectDirectAdded проверяет, что следующий кадр — chat_added лички с peer.
func (c *wsConn) expectDirectAdded(t *testing.T, chatID int, peer *user) {
	t.Helper()

	f := c.next(t)
	if f.Type != "chat_added" {
		t.Fatalf("ждали chat_added, пришёл %s %s", f.Type, f.Payload)
	}

	var added struct {
		ChatID int    `json:"chat_id"`
		Kind   string `json:"kind"`
		PeerID int    `json:"peer_id"`
	}
	if err := json.Unmarshal(f.Payload, &added); err != nil {
		t.Fatal(err)
	}

	if added.ChatID != chatID || added.Kind != "direct" || added.PeerID != peer.id {
		t.Fatalf("chat_added %s, ждали личку %d с %d", f.Payload, chatID, peer.id)
	}
}

func countRows(t *testing.T, query string, args ...any) int {
	t.Helper()

	var n int
	if err := testPool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}

	return n
}

func imagesInChat(t *testing.T, chatID int) int {
	t.Helper()

	return countRows(t, `SELECT count(*) FROM images WHERE chat_id = $1`, chatID)
}

// requireNoOrphans проверяет, что у каждой картинки в базе есть сообщение.
func requireNoOrphans(t *testing.T) {
	t.Helper()

	n := countRows(t, `SELECT count(*) FROM images i
		WHERE NOT EXISTS (SELECT 1 FROM messages m WHERE m.image_id = i.id)`)
	if n != 0 {
		t.Fatalf("картинок без сообщения: %d", n)
	}
}

func (e *env) history(t *testing.T, u *user, chatID int) []map[string]json.RawMessage {
	t.Helper()

	resp, body := e.get(t, u, fmt.Sprintf("/chats/%d/messages", chatID))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("история: %d %s", resp.StatusCode, body)
	}

	var items []map[string]json.RawMessage
	if err := json.Unmarshal(body, &items); err != nil {
		t.Fatalf("история не разобрана: %v; %s", err, body)
	}

	return items
}

func intField(t *testing.T, item map[string]json.RawMessage, key string) int {
	t.Helper()

	var v int
	if err := json.Unmarshal(item[key], &v); err != nil {
		t.Fatalf("поле %s не число: %s", key, item[key])
	}

	return v
}

func TestSendImageByChatID(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	chatID := e.directChat(t, a, b)

	a1, a2, bc := e.dial(t, a), e.dial(t, a), e.dial(t, b)

	sent := e.sendImage(t, a, toChat(chatID, newUUID(t), "закат"), pngFile(t))
	if sent.ChatID != chatID {
		t.Fatalf("chat_id %d, ждали %d", sent.ChatID, chatID)
	}

	for _, c := range []*wsConn{a1, a2, bc} {
		c.expectImageMessage(t, sent, a, "закат")
	}

	if n := imagesInChat(t, chatID); n != 1 {
		t.Fatalf("картинок в чате %d, ждали 1", n)
	}
}

func TestSendImageByPeerID(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	a1, a2, bc := e.dial(t, a), e.dial(t, a), e.dial(t, b)

	sent := e.sendImage(t, a, toPeer(b, newUUID(t), ""), jpegFile(t))

	// Личный чат заведён: его находит GET /chats/direct.
	resp, body := e.get(t, b, fmt.Sprintf("/chats/direct?peer_id=%d", a.id))
	if resp.StatusCode != http.StatusOK || chatIDOf(t, body) != sent.ChatID {
		t.Fatalf("личка не найдена: %d %s, ждали chat_id %d", resp.StatusCode, body, sent.ChatID)
	}

	// chat_added приходит обоим на все соединения раньше message.
	for _, c := range []*wsConn{a1, a2} {
		c.expectDirectAdded(t, sent.ChatID, b)
		c.expectImageMessage(t, sent, a, "")
	}
	bc.expectDirectAdded(t, sent.ChatID, a)
	bc.expectImageMessage(t, sent, a, "")

	// Чат уже есть: вторая картинка по peer_id идёт в него же без chat_added.
	again := e.sendImage(t, b, toPeer(a, newUUID(t), "ещё"), pngFile(t))
	if again.ChatID != sent.ChatID {
		t.Fatalf("вторая картинка ушла в чат %d, ждали %d", again.ChatID, sent.ChatID)
	}
	for _, c := range []*wsConn{a1, a2, bc} {
		c.expectImageMessage(t, again, b, "ещё")
	}
}

func TestSendImageFormatByContent(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	chatID := e.directChat(t, a, b)

	cases := []struct {
		name string
		file *filePart
		want string
	}{
		{"jpeg", jpegFile(t), "image/jpeg"},
		{"png", pngFile(t), "image/png"},
		// Имя и Content-Type части врут: формат берётся из содержимого.
		{"png под видом jpeg", &filePart{name: "photo.jpg", contentType: "image/jpeg", data: pngBytes(t)}, "image/png"},
		{"jpeg под видом png", &filePart{name: "photo.png", contentType: "image/png", data: jpegBytes(t)}, "image/jpeg"},
		{"без Content-Type части", &filePart{name: "photo", contentType: "application/octet-stream", data: jpegBytes(t)}, "image/jpeg"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sent := e.sendImage(t, a, toChat(chatID, newUUID(t), ""), tc.file)

			resp, body := e.get(t, b, fmt.Sprintf("/images/%d", sent.ImageID))
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("картинка не отдана: %d %s", resp.StatusCode, body)
			}
			if got := resp.Header.Get("Content-Type"); got != tc.want {
				t.Fatalf("Content-Type %q, ждали %q", got, tc.want)
			}
			if !bytes.Equal(body, tc.file.data) {
				t.Fatal("байты картинки не совпали с отправленными")
			}
		})
	}
}

func TestSendImageCaption(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	chatID := e.directChat(t, a, b)

	maxCaption := strings.Repeat("я", 4096)

	t.Run("пустая подпись", func(t *testing.T) {
		sent := e.sendImage(t, a, toChat(chatID, newUUID(t), ""), pngFile(t))
		requireHistoryBody(t, e.history(t, b, chatID), sent.MessageID, "")
	})

	t.Run("без поля body", func(t *testing.T) {
		fields := map[string]string{"chat_id": strconv.Itoa(chatID), "client_msg_id": newUUID(t)}
		sent := e.sendImage(t, a, fields, pngFile(t))
		requireHistoryBody(t, e.history(t, b, chatID), sent.MessageID, "")
	})

	t.Run("4096 символов кириллицей", func(t *testing.T) {
		sent := e.sendImage(t, a, toChat(chatID, newUUID(t), maxCaption), pngFile(t))
		requireHistoryBody(t, e.history(t, b, chatID), sent.MessageID, maxCaption)
	})

	t.Run("4096 символов и пробелы по краям", func(t *testing.T) {
		e.sendImage(t, a, toChat(chatID, newUUID(t), "  "+maxCaption+"\n "), pngFile(t))
	})

	t.Run("4097 символов", func(t *testing.T) {
		before := imagesInChat(t, chatID)

		status, body := e.postImage(t, a, toChat(chatID, newUUID(t), maxCaption+"я"), pngFile(t))
		requireError(t, status, body, http.StatusBadRequest)

		if n := imagesInChat(t, chatID); n != before {
			t.Fatalf("картинок в чате %d, ждали %d", n, before)
		}
	})
}

func requireHistoryBody(t *testing.T, items []map[string]json.RawMessage, messageID int, want string) {
	t.Helper()

	for _, item := range items {
		if intField(t, item, "message_id") != messageID {
			continue
		}

		raw, ok := item["body"]
		if !ok {
			t.Fatalf("в элементе истории нет body: %v", item)
		}

		var body string
		if err := json.Unmarshal(raw, &body); err != nil || body != want {
			t.Fatalf("body %s, ждали %q", raw, want)
		}
		return
	}

	t.Fatalf("сообщения %d нет в истории", messageID)
}

func TestSendImageRetry(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b, c := e.newUser(t), e.newUser(t), e.newUser(t)
	chatID := e.directChat(t, a, b)

	t.Run("по chat_id", func(t *testing.T) {
		ac, bc := e.dial(t, a), e.dial(t, b)

		fields := toChat(chatID, newUUID(t), "один раз")
		status, first := e.postImage(t, a, fields, pngFile(t))
		if status != http.StatusCreated {
			t.Fatalf("первая отправка: %d %s", status, first)
		}
		sent := decodeImageResponse(t, first)

		// Повтор с тем же client_msg_id, даже с другим файлом, ничего не сохраняет.
		status, second := e.postImage(t, a, fields, jpegFile(t))
		if status != http.StatusOK {
			t.Fatalf("повтор: %d %s, ждали 200", status, second)
		}
		requireSameJSON(t, second, first)

		if n := imagesInChat(t, chatID); n != 1 {
			t.Fatalf("картинок в чате %d, ждали 1", n)
		}
		if n := countRows(t, `SELECT count(*) FROM messages WHERE sender_id = $1 AND client_msg_id = $2`,
			a.id, fields["client_msg_id"]); n != 1 {
			t.Fatalf("сообщений с этим client_msg_id %d, ждали 1", n)
		}

		// message пришёл один раз: сразу за ним — барьер.
		barrier := e.createGroup(t, a, b)
		for _, conn := range []*wsConn{ac, bc} {
			conn.expectImageMessage(t, sent, a, "один раз")
			conn.expectBarrier(t, barrier)
		}
	})

	t.Run("по peer_id", func(t *testing.T) {
		cc := e.dial(t, c)

		fields := toPeer(c, newUUID(t), "")
		status, first := e.postImage(t, a, fields, pngFile(t))
		if status != http.StatusCreated {
			t.Fatalf("первая отправка: %d %s", status, first)
		}
		sent := decodeImageResponse(t, first)

		status, second := e.postImage(t, a, fields, pngFile(t))
		if status != http.StatusOK {
			t.Fatalf("повтор: %d %s, ждали 200", status, second)
		}
		requireSameJSON(t, second, first)

		if n := imagesInChat(t, sent.ChatID); n != 1 {
			t.Fatalf("картинок в чате %d, ждали 1", n)
		}

		// Ни второго chat_added, ни второго message.
		barrier := e.createGroup(t, a, c)
		cc.expectDirectAdded(t, sent.ChatID, a)
		cc.expectImageMessage(t, sent, a, "")
		cc.expectBarrier(t, barrier)
	})
}

func requireSameJSON(t *testing.T, got, want []byte) {
	t.Helper()

	var g, w map[string]any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("ответ не JSON: %s", got)
	}
	if err := json.Unmarshal(want, &w); err != nil {
		t.Fatalf("ответ не JSON: %s", want)
	}

	gs, _ := json.Marshal(g)
	ws, _ := json.Marshal(w)
	if !bytes.Equal(gs, ws) {
		t.Fatalf("тело повтора %s, ждали %s", got, want)
	}
}

func TestSendImageBadRequest(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b, fresh := e.newUser(t), e.newUser(t), e.newUser(t)
	chatID := e.directChat(t, a, b)
	chat := strconv.Itoa(chatID)

	cases := []struct {
		name   string
		fields map[string]string
		file   *filePart
	}{
		{"нет файла", toChat(chatID, newUUID(t), "подпись"), nil},
		{"file текстовым полем", map[string]string{"chat_id": chat, "client_msg_id": newUUID(t), "file": "картинка"}, nil},
		{"пустой файл", toChat(chatID, newUUID(t), ""), &filePart{name: "photo.jpg", contentType: "image/jpeg"}},
		{"gif под видом jpeg", toChat(chatID, newUUID(t), ""),
			&filePart{name: "photo.jpg", contentType: "image/jpeg", data: gifBytes(t)}},
		{"текст под видом jpeg", toChat(chatID, newUUID(t), ""),
			&filePart{name: "photo.jpg", contentType: "image/jpeg", data: []byte("это не картинка, а просто текст")}},
		{"нет chat_id и peer_id", map[string]string{"client_msg_id": newUUID(t)}, pngFile(t)},
		{"chat_id и peer_id вместе",
			map[string]string{"chat_id": chat, "peer_id": strconv.Itoa(b.id), "client_msg_id": newUUID(t)}, pngFile(t)},
		{"client_msg_id не UUID", toChat(chatID, "не-uuid", ""), pngFile(t)},
		{"нет client_msg_id", map[string]string{"chat_id": chat}, pngFile(t)},
		{"peer_id равен моему id", toPeer(a, newUUID(t), ""), pngFile(t)},
		// Нечисловой chat_id в таблице ответов отдельно не назван, но это тот же
		// случай, что «нет chat_id»: адресата нет.
		{"chat_id не число", map[string]string{"chat_id": "abc", "client_msg_id": newUUID(t)}, pngFile(t)},
		// Отказ по peer_id не заводит личку с собеседником.
		{"gif по peer_id", toPeer(fresh, newUUID(t), ""),
			&filePart{name: "photo.jpg", contentType: "image/jpeg", data: gifBytes(t)}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := e.postImage(t, a, tc.fields, tc.file)
			requireError(t, status, body, http.StatusBadRequest)
		})
	}

	if n := imagesInChat(t, chatID); n != 0 {
		t.Fatalf("картинок в чате %d, ждали 0", n)
	}

	resp, body := e.get(t, a, fmt.Sprintf("/chats/direct?peer_id=%d", fresh.id))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("после отказа завелась личка: %d %s", resp.StatusCode, body)
	}

	requireNoOrphans(t)
}

// Тело не multipart — 415 и тело ошибки, как у любой ручки, получившей тело
// не того типа.
func TestSendImageNotMultipart(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	chatID := e.directChat(t, a, b)

	raw, err := json.Marshal(map[string]any{"chat_id": chatID, "client_msg_id": newUUID(t)})
	if err != nil {
		t.Fatal(err)
	}

	resp, body := e.do(t, a, http.MethodPost, "/chats/images", "application/json", bytes.NewReader(raw))
	requireError(t, resp.StatusCode, body, http.StatusUnsupportedMediaType)

	resp, body = e.do(t, a, http.MethodPost, "/chats/images", "text/plain", strings.NewReader("картинка"))
	requireError(t, resp.StatusCode, body, http.StatusUnsupportedMediaType)
}

func TestSendImageForbidden(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	t.Run("не участник", func(t *testing.T) {
		a, b, stranger := e.newUser(t), e.newUser(t), e.newUser(t)
		chatID := e.directChat(t, a, b)

		status, body := e.postImage(t, stranger, toChat(chatID, newUUID(t), ""), pngFile(t))
		requireError(t, status, body, http.StatusForbidden)

		if n := imagesInChat(t, chatID); n != 0 {
			t.Fatalf("картинок в чате %d, ждали 0", n)
		}
	})

	t.Run("канал", func(t *testing.T) {
		owner, member, stranger := e.newUser(t), e.newUser(t), e.newUser(t)
		channelID := e.createChannel(t, owner)
		e.joinChannel(t, member, channelID)

		mc, sc := e.dial(t, member), e.dial(t, stranger)

		// Подписчик без роли owner/admin в канал не пишет.
		status, body := e.postImage(t, member, toChat(channelID, newUUID(t), ""), pngFile(t))
		requireError(t, status, body, http.StatusForbidden)

		if n := imagesInChat(t, channelID); n != 0 {
			t.Fatalf("картинок в канале %d, ждали 0", n)
		}

		// Посторонний — тоже 403.
		status, body = e.postImage(t, stranger, toChat(channelID, newUUID(t), ""), pngFile(t))
		requireError(t, status, body, http.StatusForbidden)

		// Владелец пишет, подписчик получает пост и видит картинку.
		sent := e.sendImage(t, owner, toChat(channelID, newUUID(t), "пост"), pngFile(t))
		mc.expectImageMessage(t, sent, owner, "пост")

		resp, body := e.get(t, member, fmt.Sprintf("/images/%d", sent.ImageID))
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("подписчик не получил картинку: %d %s", resp.StatusCode, body)
		}

		resp, body = e.get(t, stranger, fmt.Sprintf("/images/%d", sent.ImageID))
		requireError(t, resp.StatusCode, body, http.StatusNotFound)

		// Посторонний пост не получил.
		barrier := e.createGroup(t, stranger, owner)
		sc.expectBarrier(t, barrier)
	})

	requireNoOrphans(t)
}

func TestSendImageNotFound(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a := e.newUser(t)

	status, body := e.postImage(t, a, toChat(missingID, newUUID(t), ""), pngFile(t))
	requireError(t, status, body, http.StatusNotFound)

	fields := map[string]string{"peer_id": strconv.Itoa(missingID), "client_msg_id": newUUID(t)}
	status, body = e.postImage(t, a, fields, pngFile(t))
	requireError(t, status, body, http.StatusNotFound)

	requireNoOrphans(t)
}

func TestSendImageTooLarge(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	chatID := e.directChat(t, a, b)

	big := &filePart{name: "big.jpg", contentType: "image/jpeg", data: jpegOfSize(t, maxImageSize+1)}
	status, body := e.postImage(t, a, toChat(chatID, newUUID(t), ""), big)
	requireError(t, status, body, http.StatusRequestEntityTooLarge)

	if n := imagesInChat(t, chatID); n != 0 {
		t.Fatalf("картинок в чате %d, ждали 0", n)
	}

	// Ровно 5 МиБ — ещё можно.
	limit := &filePart{name: "limit.jpg", contentType: "image/jpeg", data: jpegOfSize(t, maxImageSize)}
	sent := e.sendImage(t, a, toChat(chatID, newUUID(t), ""), limit)

	resp, data := e.get(t, b, fmt.Sprintf("/images/%d", sent.ImageID))
	if resp.StatusCode != http.StatusOK || !bytes.Equal(data, limit.data) {
		t.Fatalf("картинка в 5 МиБ не отдана целиком: %d, %d байт", resp.StatusCode, len(data))
	}
}

func TestImagesUnauthorized(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	chatID := e.directChat(t, a, b)
	sent := e.sendImage(t, a, toChat(chatID, newUUID(t), ""), pngFile(t))

	status, body := e.postImage(t, nil, toChat(chatID, newUUID(t), ""), pngFile(t))
	requireError(t, status, body, http.StatusUnauthorized)

	bogus := &user{cookie: &http.Cookie{Name: "token", Value: "нет-такого-токена"}}
	status, body = e.postImage(t, bogus, toChat(chatID, newUUID(t), ""), pngFile(t))
	requireError(t, status, body, http.StatusUnauthorized)

	resp, body := e.get(t, nil, fmt.Sprintf("/images/%d", sent.ImageID))
	requireError(t, resp.StatusCode, body, http.StatusUnauthorized)

	resp, body = e.get(t, bogus, fmt.Sprintf("/images/%d", sent.ImageID))
	requireError(t, resp.StatusCode, body, http.StatusUnauthorized)

	if n := imagesInChat(t, chatID); n != 1 {
		t.Fatalf("картинок в чате %d, ждали 1", n)
	}
}

func TestSendImageBroadcast(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b, c, stranger := e.newUser(t), e.newUser(t), e.newUser(t), e.newUser(t)
	groupID := e.createGroup(t, a, b, c)

	a1, a2 := e.dial(t, a), e.dial(t, a)
	bc, cc, sc := e.dial(t, b), e.dial(t, c), e.dial(t, stranger)

	sent := e.sendImage(t, a, toChat(groupID, newUUID(t), "всем"), pngFile(t))

	// Все участники, включая другое соединение отправителя.
	for _, conn := range []*wsConn{a1, a2, bc, cc} {
		conn.expectImageMessage(t, sent, a, "всем")
	}

	barrier := e.createGroup(t, stranger, a)
	sc.expectBarrier(t, barrier)
}

func TestGetImage(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b, stranger := e.newUser(t), e.newUser(t), e.newUser(t)
	chatID := e.directChat(t, a, b)

	file := pngFile(t)
	sent := e.sendImage(t, a, toChat(chatID, newUUID(t), ""), file)
	path := fmt.Sprintf("/images/%d", sent.ImageID)

	for _, u := range []*user{a, b} {
		resp, body := e.get(t, u, path)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("картинка не отдана: %d %s", resp.StatusCode, body)
		}
		if !bytes.Equal(body, file.data) {
			t.Fatal("байты картинки не совпали с отправленными")
		}
		if got := resp.Header.Get("Content-Type"); got != "image/png" {
			t.Fatalf("Content-Type %q, ждали image/png", got)
		}
		if got := resp.Header.Values("Cache-Control"); len(got) != 1 || got[0] != cacheControl {
			t.Fatalf("Cache-Control %q, ждали %q", got, cacheControl)
		}
	}

	notFound := []struct {
		name string
		u    *user
		path string
	}{
		{"не участник", stranger, path},
		{"нет такой картинки", a, fmt.Sprintf("/images/%d", missingID)},
		{"id не число", a, "/images/abc"},
	}

	for _, tc := range notFound {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := e.get(t, tc.u, tc.path)
			requireError(t, resp.StatusCode, body, http.StatusNotFound)
		})
	}
}

func TestHistoryWithImages(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	chatID := e.directChat(t, a, b)

	captioned := e.sendImage(t, a, toChat(chatID, newUUID(t), "закат"), pngFile(t))
	bare := e.sendImage(t, b, toChat(chatID, newUUID(t), ""), jpegFile(t))

	items := e.history(t, a, chatID)
	if len(items) != 3 {
		t.Fatalf("в истории %d сообщений, ждали 3", len(items))
	}

	// Первое — текст, с которого началась личка: без image_id.
	if _, ok := items[0]["image_id"]; ok {
		t.Fatalf("у текстового сообщения есть image_id: %v", items[0])
	}

	want := []struct {
		sent   imageResponse
		sender *user
		body   string
	}{
		{captioned, a, "закат"},
		{bare, b, ""},
	}

	for i, w := range want {
		item := items[i+1]

		if intField(t, item, "message_id") != w.sent.MessageID ||
			intField(t, item, "chat_id") != chatID ||
			intField(t, item, "sender_id") != w.sender.id {
			t.Fatalf("не то сообщение в истории: %v", item)
		}
		if _, ok := item["image_id"]; !ok || intField(t, item, "image_id") != w.sent.ImageID {
			t.Fatalf("image_id %s, ждали %d", item["image_id"], w.sent.ImageID)
		}

		var tm string
		if err := json.Unmarshal(item["time"], &tm); err != nil {
			t.Fatal(err)
		}
		requireUTC(t, tm)
	}

	requireHistoryBody(t, items, captioned.MessageID, "закат")
	requireHistoryBody(t, items, bare.MessageID, "")
}

func TestDeleteChatRemovesImages(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	first := e.sendImage(t, a, toPeer(b, newUUID(t), ""), pngFile(t))
	second := e.sendImage(t, b, toPeer(a, newUUID(t), "ответ"), jpegFile(t))
	chatID := first.ChatID

	if n := imagesInChat(t, chatID); n != 2 {
		t.Fatalf("картинок в чате %d, ждали 2", n)
	}

	resp, body := e.do(t, b, http.MethodPost, fmt.Sprintf("/chats/%d/delete", chatID), "", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("удаление чата: %d %s", resp.StatusCode, body)
	}

	for _, u := range []*user{a, b} {
		for _, sent := range []imageResponse{first, second} {
			resp, body = e.get(t, u, fmt.Sprintf("/images/%d", sent.ImageID))
			requireError(t, resp.StatusCode, body, http.StatusNotFound)
		}
	}

	if n := imagesInChat(t, chatID); n != 0 {
		t.Fatalf("после удаления чата картинок %d, ждали 0", n)
	}
}

func TestSendImageConcurrentRetry(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	const requests = 8

	t.Run("по chat_id", func(t *testing.T) {
		a, b := e.newUser(t), e.newUser(t)
		chatID := e.directChat(t, a, b)
		bc := e.dial(t, b)

		fields := toChat(chatID, newUUID(t), "один")
		first := concurrentImages(t, e, a, fields, requests)

		if n := imagesInChat(t, chatID); n != 1 {
			t.Fatalf("картинок в чате %d, ждали 1", n)
		}
		if n := countRows(t, `SELECT count(*) FROM messages WHERE sender_id = $1 AND client_msg_id = $2`,
			a.id, fields["client_msg_id"]); n != 1 {
			t.Fatalf("сообщений с этим client_msg_id %d, ждали 1", n)
		}

		barrier := e.createGroup(t, a, b)
		bc.expectImageMessage(t, first, a, "один")
		bc.expectBarrier(t, barrier)
	})

	t.Run("по peer_id", func(t *testing.T) {
		a, b := e.newUser(t), e.newUser(t)
		bc := e.dial(t, b)

		fields := toPeer(b, newUUID(t), "")
		first := concurrentImages(t, e, a, fields, requests)

		if n := imagesInChat(t, first.ChatID); n != 1 {
			t.Fatalf("картинок в чате %d, ждали 1", n)
		}

		// Одна личка, один chat_added, один message.
		barrier := e.createGroup(t, a, b)
		bc.expectDirectAdded(t, first.ChatID, a)
		bc.expectImageMessage(t, first, a, "")
		bc.expectBarrier(t, barrier)
	})

	requireNoOrphans(t)
}

// concurrentImages отправляет одну и ту же картинку n раз одновременно. Ровно
// один ответ — 201, остальные — 200 с тем же телом.
func concurrentImages(t *testing.T, e *env, u *user, fields map[string]string, n int) imageResponse {
	t.Helper()

	body, contentType := imageForm(t, fields, pngFile(t))

	type result struct {
		status int
		body   []byte
		err    error
	}

	results := make([]result, n)
	start := make(chan struct{})
	var wg sync.WaitGroup

	for i := range n {
		wg.Go(func() {
			<-start

			req, err := http.NewRequest(http.MethodPost, e.srv.URL+"/chats/images", bytes.NewReader(body))
			if err != nil {
				results[i].err = err
				return
			}
			req.Header.Set("Content-Type", contentType)
			req.AddCookie(&http.Cookie{Name: u.cookie.Name, Value: u.cookie.Value})

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				results[i].err = err
				return
			}
			defer func() {
				_ = resp.Body.Close()
			}()

			var buf bytes.Buffer
			_, results[i].err = buf.ReadFrom(resp.Body)
			results[i].status = resp.StatusCode
			results[i].body = buf.Bytes()
		})
	}

	close(start)
	wg.Wait()

	var created []byte
	for _, r := range results {
		if r.err != nil {
			t.Fatal(r.err)
		}
		switch r.status {
		case http.StatusCreated:
			if created != nil {
				t.Fatalf("201 пришёл больше одного раза: %s", r.body)
			}
			created = r.body
		case http.StatusOK:
		default:
			t.Fatalf("статус %d: %s", r.status, r.body)
		}
	}
	if created == nil {
		t.Fatal("ни один запрос не получил 201")
	}

	for _, r := range results {
		requireSameJSON(t, r.body, created)
	}

	return decodeImageResponse(t, created)
}

// Обычный текст через сокет работает как раньше: ack и message без image_id.
func TestSendTextHasNoImage(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	chatID := e.directChat(t, a, b)

	ac, bc := e.dial(t, a), e.dial(t, b)

	clientMsgID := newUUID(t)
	ac.write(t, map[string]any{"type": "send", "payload": map[string]any{
		"chat_id": chatID, "body": "просто текст", "client_msg_id": clientMsgID,
	}})

	f := ac.next(t)
	if f.Type != "ack" {
		t.Fatalf("ждали ack, пришёл %s %s", f.Type, f.Payload)
	}

	var ack ackPayload
	if err := json.Unmarshal(f.Payload, &ack); err != nil {
		t.Fatal(err)
	}
	if ack.ClientMsgID != clientMsgID || ack.ChatID != chatID || ack.MessageID == 0 {
		t.Fatalf("ack %s", f.Payload)
	}
	requireUTC(t, ack.Time)

	for _, conn := range []*wsConn{ac, bc} {
		f = conn.next(t)
		if f.Type != "message" {
			t.Fatalf("ждали message, пришёл %s %s", f.Type, f.Payload)
		}

		var msg map[string]json.RawMessage
		if err := json.Unmarshal(f.Payload, &msg); err != nil {
			t.Fatal(err)
		}
		if _, ok := msg["image_id"]; ok {
			t.Fatalf("у текстового сообщения есть image_id: %s", f.Payload)
		}
		if intField(t, msg, "message_id") != ack.MessageID {
			t.Fatalf("message %s, ждали message_id %d", f.Payload, ack.MessageID)
		}
	}
}
