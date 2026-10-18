package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"testing"
)

// Размер пачки по умолчанию и наибольший допустимый limit.
const (
	defaultHistoryLimit = 50
	maxHistoryLimit     = 100
)

// sendMany отправляет n текстов в чат по одному, дожидаясь ack, и
// возвращает id сообщений в порядке отправки.
func (c *wsConn) sendMany(t *testing.T, chatID, n int) []int {
	t.Helper()

	ids := make([]int, 0, n)
	for i := range n {
		ack := c.sendText(t, map[string]any{"chat_id": chatID}, "сообщение "+strconv.Itoa(i))
		if ack.ChatID != chatID {
			t.Fatalf("ack в чат %d, ждали %d", ack.ChatID, chatID)
		}
		ids = append(ids, ack.MessageID)
	}

	if !slices.IsSorted(ids) {
		t.Fatalf("id сообщений не растут: %v", ids)
	}

	return ids
}

// historyQuery строит путь истории; пустое значение — параметр не передаётся.
func historyQuery(chatID int, limit, before string) string {

	q := url.Values{}
	if limit != "" {
		q.Set("limit", limit)
	}
	if before != "" {
		q.Set("before", before)
	}

	path := fmt.Sprintf("/chats/%d/messages", chatID)
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	return path
}

// page запрашивает пачку истории, ждёт 200 и возвращает id сообщений.
// Попутно проверяет, что тело — массив (не null), все элементы из этого чата
// и идут по возрастанию message_id.
func (e *env) page(t *testing.T, u *user, chatID int, path string) []int {
	t.Helper()

	resp, body := e.get(t, u, path)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s: %d %s", path, resp.StatusCode, body)
	}

	if bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		t.Fatalf("%s: пустая пачка пришла как null, а не []", path)
	}

	var items []map[string]json.RawMessage
	if err := json.Unmarshal(body, &items); err != nil {
		t.Fatalf("%s: история не разобрана: %v; %s", path, err, body)
	}

	ids := make([]int, 0, len(items))
	for _, item := range items {
		if got := intField(t, item, "chat_id"); got != chatID {
			t.Fatalf("%s: в истории сообщение чата %d: %v", path, got, item)
		}
		ids = append(ids, intField(t, item, "message_id"))
	}

	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Fatalf("%s: пачка не по возрастанию message_id: %v", path, ids)
		}
	}

	return ids
}

func requireIDs(t *testing.T, what string, got, want []int) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Fatalf("%s: пришли %v, ждали %v", what, got, want)
	}
}

// lastN — последние n элементов (все, если их меньше).
func lastN(ids []int, n int) []int {
	if n >= len(ids) {
		return ids
	}
	return ids[len(ids)-n:]
}

// beforeCursor — n сообщений с id меньше cursor, ближайших к нему.
func beforeCursor(ids []int, cursor, n int) []int {

	var older []int
	for _, id := range ids {
		if id < cursor {
			older = append(older, id)
		}
	}

	return lastN(older, n)
}

// Без параметров приходят последние 50 сообщений по возрастанию.
func TestHistoryDefaultLimit(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	chatID := e.createGroup(t, a, b)
	ids := e.dial(t, a).sendMany(t, chatID, defaultHistoryLimit+15)

	want := lastN(ids, defaultHistoryLimit)
	requireIDs(t, "без параметров", e.page(t, a, chatID, historyQuery(chatID, "", "")), want)
	requireIDs(t, "у второго участника", e.page(t, b, chatID, historyQuery(chatID, "", "")), want)
	requireIDs(t, "limit=50", e.page(t, a, chatID, historyQuery(chatID, "50", "")), want)

	// Без before, но с limit — тоже самые свежие.
	requireIDs(t, "limit=7", e.page(t, a, chatID, historyQuery(chatID, "7", "")), lastN(ids, 7))
	requireIDs(t, "limit=1", e.page(t, a, chatID, historyQuery(chatID, "1", "")), lastN(ids, 1))
}

// Листание до начала: каждая пачка — limit сообщений, ближайших к курсору;
// последняя короче limit, а при кратном числе сообщений после полной пачки
// приходит []. Склейка всех пачек — вся история без дублей и пропусков.
func TestHistoryPagingToStart(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	chatID := e.createGroup(t, a, b)
	ids := e.dial(t, a).sendMany(t, chatID, 120)

	for _, limit := range []int{maxHistoryLimit, 40, 7, 1} {
		t.Run("limit="+strconv.Itoa(limit), func(t *testing.T) {

			var all []int
			cursor := ""
			for range len(ids) + 2 {
				got := e.page(t, a, chatID, historyQuery(chatID, strconv.Itoa(limit), cursor))

				if cursor == "" {
					requireIDs(t, "первая пачка", got, lastN(ids, limit))
				} else {
					c, _ := strconv.Atoi(cursor)
					requireIDs(t, "пачка до "+cursor, got, beforeCursor(ids, c, limit))
				}

				if len(got) > limit {
					t.Fatalf("пачка длиннее limit: %d > %d", len(got), limit)
				}

				all = slices.Concat(got, all)
				if len(got) < limit {
					break
				}
				cursor = strconv.Itoa(got[0])
			}

			requireIDs(t, "склейка пачек", all, ids)

			// 120 не кратно 7 и 100: последняя пачка непустая и короче limit.
			// Для 40 и 1 кратно: после полной пачки пришёл [].
			if len(ids)%limit == 0 {
				requireIDs(t, "после начала истории", e.page(t, a, chatID, historyQuery(chatID, strconv.Itoa(limit), strconv.Itoa(ids[0]))), []int{})
			}
		})
	}
}

// Курсор — граница по id, а не номер сообщения: before может быть любым
// числом, в том числе id из другого чата или больше всех id в чате.
func TestHistoryBeforeCursor(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	chatID := e.createGroup(t, a, b)
	other := e.createGroup(t, a, b)

	ac := e.dial(t, a)
	first := ac.sendMany(t, chatID, 10)
	// Сообщение другого чата встаёт между id этого: курсор по нему тоже работает.
	gap := ac.sendMany(t, other, 1)[0]
	second := ac.sendMany(t, chatID, 10)
	ids := append(slices.Clone(first), second...)

	cases := []struct {
		name   string
		limit  string
		before int
		want   []int
	}{
		{"курсор на сообщении", "5", ids[15], ids[10:15]},
		{"курсор на соседнем чате", "3", gap, first[7:]},
		{"курсор выше всех", "4", ids[19] + 1000, ids[16:]},
		{"по умолчанию 50", "", ids[19], ids[:19]},
		{"старше нет", "5", ids[0], []int{}},
		{"меньше limit до начала", "5", ids[2], ids[:2]},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := e.page(t, a, chatID, historyQuery(chatID, tc.limit, strconv.Itoa(tc.before)))
			requireIDs(t, tc.name, got, tc.want)
		})
	}
}

// before = 0 и отрицательный: сообщений с id меньше курсора нет. Контракт
// требует 400 только для нечислового before, так что это обычная пустая пачка.
func TestHistoryBeforeNotPositive(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	chatID := e.createGroup(t, a, b)
	e.dial(t, a).sendMany(t, chatID, 3)

	for _, before := range []string{"0", "-1"} {
		got := e.page(t, a, chatID, historyQuery(chatID, "10", before))
		requireIDs(t, "before="+before, got, []int{})
	}
}

// Новые сообщения, пришедшие между запросами, не сдвигают пачки: курсор
// держится за id, а не за смещение.
func TestHistoryCursorStableWhenNewArrive(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	chatID := e.createGroup(t, a, b)
	ac := e.dial(t, a)
	ids := ac.sendMany(t, chatID, 30)

	p1 := e.page(t, b, chatID, historyQuery(chatID, "10", ""))
	requireIDs(t, "первая пачка", p1, ids[20:])

	fresh := ac.sendMany(t, chatID, 5)

	p2 := e.page(t, b, chatID, historyQuery(chatID, "10", strconv.Itoa(p1[0])))
	requireIDs(t, "вторая пачка", p2, ids[10:20])

	ac.sendMany(t, chatID, 5)

	p3 := e.page(t, b, chatID, historyQuery(chatID, "10", strconv.Itoa(p2[0])))
	requireIDs(t, "третья пачка", p3, ids[:10])

	p4 := e.page(t, b, chatID, historyQuery(chatID, "10", strconv.Itoa(p3[0])))
	requireIDs(t, "после начала", p4, []int{})

	all := slices.Concat(p3, p2, p1)
	requireIDs(t, "склейка пачек", all, ids)

	// Новый запрос без курсора видит новые сообщения.
	latest := e.page(t, b, chatID, historyQuery(chatID, "10", ""))
	if !slices.Contains(latest, fresh[0]) {
		t.Fatalf("свежая пачка %v без нового сообщения %d", latest, fresh[0])
	}
}

// limit вне 1..100, нечисловой limit или before — 400 с текстом ошибки, без
// тихого урезания. Границы диапазона допустимы.
func TestHistoryBadParams(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	chatID := e.createGroup(t, a, b)
	e.dial(t, a).sendMany(t, chatID, 3)

	base := fmt.Sprintf("/chats/%d/messages", chatID)

	bad := []string{
		"?limit=0",
		"?limit=101",
		"?limit=200",
		"?limit=-1",
		"?limit=abc",
		"?limit=",
		"?limit=1.5",
		"?limit=10abc",
		"?before=abc",
		"?before=",
		"?before=1.5",
		"?limit=10&before=abc",
		"?limit=abc&before=10",
		"?limit=101&before=10",
	}

	for _, query := range bad {
		t.Run(query, func(t *testing.T) {
			resp, body := e.get(t, a, base+query)
			requireError(t, resp.StatusCode, body, http.StatusBadRequest)
		})
	}

	for _, limit := range []string{"1", "100"} {
		got := e.page(t, a, chatID, historyQuery(chatID, limit, ""))
		if len(got) == 0 {
			t.Fatalf("limit=%s: пустая пачка в непустом чате", limit)
		}
	}
}

// Пустой чат — именно [], в том числе с параметрами.
func TestHistoryEmptyChat(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	chatID := e.createGroup(t, a, b)

	for _, path := range []string{
		historyQuery(chatID, "", ""),
		historyQuery(chatID, "10", ""),
		historyQuery(chatID, "10", "1000"),
	} {
		resp, body := e.get(t, a, path)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: %d %s", path, resp.StatusCode, body)
		}
		if string(bytes.TrimSpace(body)) != "[]" {
			t.Fatalf("%s: пустая история %q, ждали []", path, body)
		}
	}
}

// Элемент пачки — та же форма, что payload кадра message: те же ключи и
// значения, у картинки ещё image_id.
func TestHistoryItemShape(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	chatID := e.createGroup(t, a, b)

	ac, bc := e.dial(t, a), e.dial(t, b)

	text := ac.sendText(t, map[string]any{"chat_id": chatID}, "привет")
	textFrame := bc.nextMessage(t)

	img := e.sendImage(t, b, toChat(chatID, newUUID(t), "закат"), pngFile(t))
	imgFrame := bc.nextMessage(t)

	resp, body := e.get(t, a, historyQuery(chatID, "", ""))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("история: %d %s", resp.StatusCode, body)
	}

	var items []json.RawMessage
	if err := json.Unmarshal(body, &items); err != nil || len(items) != 2 {
		t.Fatalf("история %s, ждали два сообщения", body)
	}

	requireSameKeysAndValues(t, items[0], textFrame)
	requireSameKeysAndValues(t, items[1], imgFrame)

	var first, second messagePayload
	if err := json.Unmarshal(items[0], &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(items[1], &second); err != nil {
		t.Fatal(err)
	}

	if first.MessageID != text.MessageID || first.ChatID != chatID || first.SenderID != a.id ||
		first.Body == nil || *first.Body != "привет" || first.ImageID != nil {
		t.Fatalf("текст в истории: %s", items[0])
	}
	requireUTC(t, first.Time)

	if second.MessageID != img.MessageID || second.SenderID != b.id ||
		second.ImageID == nil || *second.ImageID != img.ImageID ||
		second.Body == nil || *second.Body != "закат" {
		t.Fatalf("картинка в истории: %s", items[1])
	}
	requireUTC(t, second.Time)

	// id — числа, а не строки.
	var raw map[string]any
	if err := json.Unmarshal(items[1], &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"message_id", "chat_id", "sender_id", "image_id"} {
		if _, ok := raw[key].(float64); !ok {
			t.Fatalf("%s не число: %s", key, items[1])
		}
	}
}

// nextMessage ждёт следующий кадр message и отдаёт его payload.
func (c *wsConn) nextMessage(t *testing.T) json.RawMessage {
	t.Helper()

	for {
		f := c.next(t)
		switch f.Type {
		case "message":
			return f.Payload
		case "error":
			t.Fatalf("пришёл error: %s", f.Payload)
		}
	}
}

func requireSameKeysAndValues(t *testing.T, item, frame json.RawMessage) {
	t.Helper()

	var got, want map[string]any
	if err := json.Unmarshal(item, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(frame, &want); err != nil {
		t.Fatal(err)
	}

	gs, _ := json.Marshal(got)
	ws, _ := json.Marshal(want)
	if !bytes.Equal(gs, ws) {
		t.Fatalf("элемент истории %s, кадр message %s", item, frame)
	}
}

// 401 без сессии, 403 не участнику, 404 несуществующему чату — и с
// параметрами пагинации.
func TestHistoryAccess(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b, stranger := e.newUser(t), e.newUser(t), e.newUser(t)
	chatID := e.createGroup(t, a, b)
	e.dial(t, a).sendMany(t, chatID, 3)

	for _, query := range []string{"", "?limit=10", "?limit=10&before=1000000"} {
		t.Run("query="+query, func(t *testing.T) {
			path := fmt.Sprintf("/chats/%d/messages%s", chatID, query)

			resp, body := e.get(t, nil, path)
			requireError(t, resp.StatusCode, body, http.StatusUnauthorized)

			resp, body = e.get(t, stranger, path)
			requireError(t, resp.StatusCode, body, http.StatusForbidden)

			resp, body = e.get(t, a, fmt.Sprintf("/chats/%d/messages%s", missingID, query))
			requireError(t, resp.StatusCode, body, http.StatusNotFound)
		})
	}
}

// Граница clear действует в любой пачке, в том числе при before выше
// границы. У собеседника история целая.
func TestHistoryClearBorder(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	ac := e.dial(t, a)

	start := ac.sendText(t, map[string]any{"peer_id": b.id}, "привет")
	chatID := start.ChatID
	old := append([]int{start.MessageID}, ac.sendMany(t, chatID, 9)...)

	resp, body := e.do(t, a, http.MethodPost, fmt.Sprintf("/chats/%d/clear", chatID), "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear: %d %s", resp.StatusCode, body)
	}
	var cleared struct {
		UpTo int `json:"up_to_message_id"`
	}
	if err := json.Unmarshal(body, &cleared); err != nil || cleared.UpTo != old[len(old)-1] {
		t.Fatalf("clear ответил %s, ждали границу %d", body, old[len(old)-1])
	}

	fresh := ac.sendMany(t, chatID, 6)
	all := slices.Concat(old, fresh)

	cases := []struct {
		name   string
		u      *user
		limit  string
		before int
		want   []int
	}{
		{"без курсора", a, "", 0, fresh},
		{"limit больше остатка", a, "100", 0, fresh},
		{"курсор выше всех", a, "100", fresh[5] + 1, fresh},
		{"курсор внутри новых", a, "3", fresh[5], fresh[2:5]},
		{"пачка упирается в границу", a, "5", fresh[2], fresh[:2]},
		{"курсор сразу над границей", a, "5", fresh[0], []int{}},
		{"курсор на границе", a, "5", cleared.UpTo, []int{}},
		{"курсор внутри очищенного", a, "5", old[5], []int{}},

		{"собеседник: свежие", b, "100", 0, all},
		{"собеседник: курсор внутри старых", b, "3", old[5], old[2:5]},
		{"собеседник: курсор на границе", b, "100", cleared.UpTo, old[:len(old)-1]},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := ""
			if tc.before != 0 {
				before = strconv.Itoa(tc.before)
			}
			got := e.page(t, tc.u, chatID, historyQuery(chatID, tc.limit, before))
			requireIDs(t, tc.name, got, tc.want)
		})
	}

	// Листание до начала у очистившего: только новые сообщения, затем [].
	var paged []int
	cursor := ""
	for range len(all) {
		got := e.page(t, a, chatID, historyQuery(chatID, "2", cursor))
		paged = slices.Concat(got, paged)
		if len(got) < 2 {
			break
		}
		cursor = strconv.Itoa(got[0])
	}
	requireIDs(t, "склейка после clear", paged, fresh)
}

func lastRead(t *testing.T, chatID int, u *user) int {
	t.Helper()

	var v *int
	err := testPool.QueryRow(context.Background(),
		`SELECT last_read FROM users_chats WHERE chat_id = $1 AND user_id = $2`, chatID, u.id).Scan(&v)
	if err != nil {
		t.Fatal(err)
	}
	if v == nil {
		return 0
	}

	return *v
}

// Загрузка старой пачки не откатывает last_read назад.
func TestHistoryOldPageKeepsLastRead(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b := e.newUser(t), e.newUser(t)
	chatID := e.createGroup(t, a, b)
	ids := e.dial(t, a).sendMany(t, chatID, 20)

	latest := e.page(t, b, chatID, historyQuery(chatID, "5", ""))
	requireIDs(t, "свежая пачка", latest, ids[15:])
	if got := lastRead(t, chatID, b); got != ids[19] {
		t.Fatalf("last_read после свежей пачки %d, ждали %d", got, ids[19])
	}

	older := e.page(t, b, chatID, historyQuery(chatID, "5", strconv.Itoa(latest[0])))
	requireIDs(t, "старая пачка", older, ids[10:15])
	if got := lastRead(t, chatID, b); got != ids[19] {
		t.Fatalf("last_read после старой пачки %d, ждали %d", got, ids[19])
	}

	e.page(t, b, chatID, historyQuery(chatID, "5", strconv.Itoa(ids[0]+1)))
	if got := lastRead(t, chatID, b); got != ids[19] {
		t.Fatalf("last_read после самой старой пачки %d, ждали %d", got, ids[19])
	}
}

// Сообщения других чатов в выдачу не попадают, даже вперемешку по id.
func TestHistoryIsolation(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	a, b, c := e.newUser(t), e.newUser(t), e.newUser(t)
	first := e.createGroup(t, a, b)
	second := e.createGroup(t, a, b, c)

	ac := e.dial(t, a)

	var firstIDs, secondIDs []int
	for range 8 {
		firstIDs = append(firstIDs, ac.sendMany(t, first, 1)...)
		secondIDs = append(secondIDs, ac.sendMany(t, second, 2)...)
	}

	requireIDs(t, "первый чат", e.page(t, a, first, historyQuery(first, "100", "")), firstIDs)
	requireIDs(t, "второй чат", e.page(t, a, second, historyQuery(second, "100", "")), secondIDs)

	// Курсор из чужого чата: пачка всё равно только из своего.
	requireIDs(t, "первый чат с курсором", e.page(t, a, first, historyQuery(first, "3", strconv.Itoa(secondIDs[10]))),
		beforeCursor(firstIDs, secondIDs[10], 3))

	// Участник одного чата не видит второй.
	resp, body := e.get(t, c, historyQuery(first, "100", ""))
	requireError(t, resp.StatusCode, body, http.StatusForbidden)
}
