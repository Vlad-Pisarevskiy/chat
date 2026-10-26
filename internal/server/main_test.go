// Отключено, пока тесты не переведены на сервис auth: регистрации и логина в chat больше нет.
//go:build ignore

package server_test

import (
	"bytes"
	"chatflow/internal/hub"
	"chatflow/internal/repository/postgres"
	"chatflow/internal/server"
	"chatflow/internal/service"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

const (
	testPassword = "пароль123"

	// Сколько ждать кадра из сокета. С запасом: тесты гоняются с -race.
	frameTimeout = time.Second * 10
)

// testPool — настоящая тестовая база. nil, если TEST_DATABASE_URL не задан.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		// Без базы тесты пропускаются сами, см. requireDB.
		return m.Run()
	}

	// Роутер грузит шаблоны web/*.html относительными путями.
	root, err := moduleRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err = os.Chdir(root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if err = resetSchema(dsn, filepath.Join(root, "migrations")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	testPool, err = pgxpool.New(context.Background(), dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer testPool.Close()

	gin.SetMode(gin.TestMode)
	gin.DefaultWriter = io.Discard

	return m.Run()
}

// moduleRoot ищет корень модуля: ближайший сверху каталог с go.mod.
func moduleRoot() (string, error) {

	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("рабочий каталог: %w", err)
	}

	for {
		if _, err = os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod не найден")
		}
		dir = parent
	}
}

// resetSchema пересоздаёт схему с нуля и накатывает миграции: каждый прогон
// начинается с чистой базы.
func resetSchema(dsn, migrations string) error {

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("подключение к тестовой базе: %w", err)
	}
	defer func() {
		_ = db.Close()
	}()

	for _, query := range []string{`DROP SCHEMA IF EXISTS public CASCADE`, `CREATE SCHEMA public`} {
		if _, err = db.Exec(query); err != nil {
			return fmt.Errorf("пересоздание схемы: %w", err)
		}
	}

	goose.SetLogger(goose.NopLogger())
	if err = goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("диалект goose: %w", err)
	}
	if err = goose.Up(db, migrations); err != nil {
		return fmt.Errorf("миграции: %w", err)
	}

	return nil
}

// requireDB пропускает тест без базы. В CI база обязана быть.
func requireDB(t *testing.T) {
	t.Helper()

	if testPool != nil {
		return
	}

	if os.Getenv("CI") != "" {
		t.Fatal("TEST_DATABASE_URL не задан")
	}
	t.Skip("TEST_DATABASE_URL не задан")
}

// env — сервер с настоящим роутером поверх тестовой базы. У каждого теста свой
// hub, так что кадры соседних тестов сюда не попадают.
type env struct {
	srv *httptest.Server
}

func newEnv(t *testing.T) *env {
	t.Helper()
	requireDB(t)

	s := server.New(service.New(postgres.New(testPool)), hub.New())
	srv := httptest.NewServer(s.GetRouter())
	t.Cleanup(srv.Close)

	return &env{srv: srv}
}

type user struct {
	id     int
	login  string
	cookie *http.Cookie
}

// newUser регистрирует и логинит пользователя через ручки. Логин уникален,
// поэтому тесты не мешают друг другу и повторному прогону.
func (e *env) newUser(t *testing.T) *user {
	t.Helper()

	login := "u_" + randomHex(t, 10)

	status, body := e.postJSON(t, nil, "/auth/register", map[string]string{
		"name":     "Тест " + login,
		"login":    login,
		"password": testPassword,
	})
	if status != http.StatusCreated {
		t.Fatalf("регистрация: %d %s", status, body)
	}

	raw, err := json.Marshal(map[string]string{"login": login, "password": testPassword})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.Post(e.srv.URL+"/auth/login", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("вход: %d", resp.StatusCode)
	}

	u := &user{login: login}
	for _, c := range resp.Cookies() {
		if c.Name == "token" {
			u.cookie = c
		}
	}
	if u.cookie == nil {
		t.Fatal("вход не поставил куку сессии")
	}

	// Ответ входа id не содержит, берём его из базы.
	if err = testPool.QueryRow(context.Background(), `SELECT id FROM users WHERE login = $1`, login).Scan(&u.id); err != nil {
		t.Fatal(err)
	}

	return u
}

// do выполняет запрос от имени пользователя; u == nil — без сессии. Кука
// ставится руками: она Secure, и cookiejar не отдал бы её по http.
func (e *env) do(t *testing.T, u *user, method, path, contentType string, body io.Reader) (*http.Response, []byte) {
	t.Helper()

	req, err := http.NewRequest(method, e.srv.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if u != nil {
		req.AddCookie(&http.Cookie{Name: u.cookie.Name, Value: u.cookie.Value})
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	return resp, data
}

func (e *env) postJSON(t *testing.T, u *user, path string, payload any) (int, []byte) {
	t.Helper()

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	resp, body := e.do(t, u, http.MethodPost, path, "application/json", bytes.NewReader(raw))
	return resp.StatusCode, body
}

func (e *env) get(t *testing.T, u *user, path string) (*http.Response, []byte) {
	t.Helper()

	return e.do(t, u, http.MethodGet, path, "", nil)
}

// createGroup создаёт группу; создатель — owner, members — остальные участники.
func (e *env) createGroup(t *testing.T, owner *user, members ...*user) int {
	t.Helper()

	ids := make([]int, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.id)
	}

	status, body := e.postJSON(t, owner, "/chats/group", map[string]any{
		"name":       "Группа",
		"member_ids": ids,
	})
	if status != http.StatusCreated {
		t.Fatalf("создание группы: %d %s", status, body)
	}

	return chatIDOf(t, body)
}

// createChannel создаёт публичный канал, создатель становится owner.
func (e *env) createChannel(t *testing.T, owner *user) int {
	t.Helper()

	status, body := e.postJSON(t, owner, "/chats/channels", map[string]any{
		"name":   "Канал",
		"handle": "ch" + randomHex(t, 8),
	})
	if status != http.StatusCreated {
		t.Fatalf("создание канала: %d %s", status, body)
	}

	return chatIDOf(t, body)
}

func (e *env) joinChannel(t *testing.T, u *user, chatID int) {
	t.Helper()

	resp, body := e.do(t, u, http.MethodPost, fmt.Sprintf("/chats/%d/join", chatID), "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("вступление в канал: %d %s", resp.StatusCode, body)
	}
}

// directChat заводит личку первым текстовым сообщением по peer_id через сокет.
func (e *env) directChat(t *testing.T, from, to *user) int {
	t.Helper()

	c := e.dial(t, from)
	defer c.close()

	ack := c.sendText(t, map[string]any{"peer_id": to.id}, "привет")
	return ack.ChatID
}

func chatIDOf(t *testing.T, body []byte) int {
	t.Helper()

	var resp struct {
		ChatID int `json:"chat_id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.ChatID == 0 {
		t.Fatalf("в ответе нет chat_id: %s", body)
	}

	return resp.ChatID
}

// requireError проверяет статус и тело ошибки: JSON с непустым ключом error.
func requireError(t *testing.T, status int, body []byte, want int) {
	t.Helper()

	if status != want {
		t.Fatalf("статус %d, ждали %d; тело %s", status, want, body)
	}

	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("тело ошибки не JSON: %q", body)
	}

	text, ok := resp["error"].(string)
	if !ok || text == "" {
		t.Fatalf("в теле ошибки нет текста под ключом error: %s", body)
	}
}

func randomHex(t *testing.T, n int) string {
	t.Helper()

	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}

	return hex.EncodeToString(b)
}

// wsConn — WebSocket-соединение пользователя с тестовым сервером.
type wsConn struct {
	conn *websocket.Conn
}

type frame struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type ackPayload struct {
	ClientMsgID string `json:"client_msg_id"`
	MessageID   int    `json:"message_id"`
	ChatID      int    `json:"chat_id"`
	Time        string `json:"time"`
}

// dial открывает сокет без Origin и ждёт, пока сервер зарегистрирует
// соединение: на неизвестный кадр приходит error, а читать кадры сессия
// начинает только после регистрации в hub.
func (e *env) dial(t *testing.T, u *user) *wsConn {
	t.Helper()

	url := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/ws/"
	header := http.Header{"Cookie": {u.cookie.Name + "=" + u.cookie.Value}}

	conn, resp, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		t.Fatalf("сокет не открылся: %v", err)
	}
	_ = resp.Body.Close()

	c := &wsConn{conn: conn}
	t.Cleanup(c.close)

	c.write(t, map[string]any{"type": "sync"})
	if f := c.next(t); f.Type != "error" {
		t.Fatalf("ждали error на неизвестный кадр, пришёл %s %s", f.Type, f.Payload)
	}

	return c
}

func (c *wsConn) close() {
	_ = c.conn.Close()
}

func (c *wsConn) write(t *testing.T, v any) {
	t.Helper()

	if err := c.conn.WriteJSON(v); err != nil {
		t.Fatal(err)
	}
}

// next читает следующий кадр, пропуская presence: он приходит всем
// подключённым и к проверкам отношения не имеет.
func (c *wsConn) next(t *testing.T) frame {
	t.Helper()

	for {
		if err := c.conn.SetReadDeadline(time.Now().Add(frameTimeout)); err != nil {
			t.Fatal(err)
		}

		_, data, err := c.conn.ReadMessage()
		if err != nil {
			t.Fatalf("кадр не пришёл: %v", err)
		}

		var f frame
		if err = json.Unmarshal(data, &f); err != nil {
			t.Fatalf("кадр не разобран: %s", data)
		}

		if f.Type != "presence" {
			return f
		}
	}
}

// sendText отправляет текст через send и ждёт ack.
func (c *wsConn) sendText(t *testing.T, target map[string]any, body string) ackPayload {
	t.Helper()

	payload := map[string]any{"body": body, "client_msg_id": newUUID(t)}
	for k, v := range target {
		payload[k] = v
	}

	c.write(t, map[string]any{"type": "send", "payload": payload})

	for {
		f := c.next(t)
		switch f.Type {
		case "ack":
			var ack ackPayload
			if err := json.Unmarshal(f.Payload, &ack); err != nil {
				t.Fatal(err)
			}
			return ack
		case "error":
			t.Fatalf("send отклонён: %s", f.Payload)
		}
	}
}

// expectBarrier проверяет, что следующий кадр — chat_added барьерной группы.
// Кадры в соединение идут по порядку, так что всё, что сервер разослал до
// создания барьера, пришло бы раньше него.
func (c *wsConn) expectBarrier(t *testing.T, chatID int) {
	t.Helper()

	f := c.next(t)
	if f.Type != "chat_added" {
		t.Fatalf("ждали chat_added барьера, пришёл %s %s", f.Type, f.Payload)
	}

	var added struct {
		ChatID int `json:"chat_id"`
	}
	if err := json.Unmarshal(f.Payload, &added); err != nil || added.ChatID != chatID {
		t.Fatalf("ждали chat_added барьера %d, пришёл %s", chatID, f.Payload)
	}
}
