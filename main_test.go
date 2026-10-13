package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func request(t *testing.T, h http.Handler, method, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "/api/todos", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// REQ-01: 루트 화면에 한국어 입력, 추가 버튼, 목록 영역을 제공한다.
func TestHomePage(t *testing.T) {
	w := httptest.NewRecorder()
	(&todoStore{}).handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "할 일을 입력하세요") || !strings.Contains(w.Body.String(), "추가") || !strings.Contains(w.Body.String(), `id="todos"`) {
		t.Fatalf("home page status/content unexpected: %d", w.Code)
	}
}

// REQ-02: 유효한 제목을 추가하고 식별자와 제목을 돌려준다.
func TestCreateTodo(t *testing.T) {
	h := (&todoStore{}).handler()
	w := request(t, h, http.MethodPost, `{"title":" 회의 준비 "}`)
	var got Todo
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusCreated || got.ID != 1 || got.Title != "회의 준비" {
		t.Fatalf("got %d %#v", w.Code, got)
	}
	bad := request(t, h, http.MethodPost, `{"title":`)
	if bad.Code != http.StatusBadRequest || !strings.Contains(bad.Body.String(), "요청 형식이 올바르지 않습니다.") {
		t.Fatalf("bad JSON: %d %s", bad.Code, bad.Body)
	}
}

// REQ-02: 문자열이 아닌 제목은 요청 형식 오류로 거절하고 목록을 바꾸지 않는다.
func TestCreateRejectsNonStringTitle(t *testing.T) {
	h := (&todoStore{}).handler()
	for _, body := range []string{`{"title":null}`, `{"title":123}`} {
		w := request(t, h, http.MethodPost, body)
		if w.Code != http.StatusBadRequest || strings.TrimSpace(w.Body.String()) != `{"error":"요청 형식이 올바르지 않습니다."}` {
			t.Fatalf("%s: got %d %s", body, w.Code, w.Body)
		}
	}
	if got := request(t, h, http.MethodGet, "").Body.String(); !strings.Contains(got, `"todos":[]`) {
		t.Fatalf("list changed: %s", got)
	}
}

// REQ-03: 비어 있거나 공백뿐인 제목은 거절하고 목록을 바꾸지 않는다.
func TestRejectsBlankTitle(t *testing.T) {
	h := (&todoStore{}).handler()
	for _, body := range []string{`{"title":""}`, `{"title":"   "}`, `{}`} {
		w := request(t, h, http.MethodPost, body)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "제목을 입력해 주세요.") {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body)
		}
	}
	if got := request(t, h, http.MethodGet, "").Body.String(); !strings.Contains(got, `"todos":[]`) {
		t.Fatalf("list changed: %s", got)
	}
}

// REQ-03 #27: 공백 문자만 있는 제목은 거절하고 목록을 바꾸지 않는다.
func TestRejectsWhitespaceOnlyTitleIssue27(t *testing.T) {
	h := (&todoStore{}).handler()
	w := request(t, h, http.MethodPost, `{"title":"   "}`)
	if w.Code != http.StatusBadRequest || strings.TrimSpace(w.Body.String()) != `{"error":"제목을 입력해 주세요."}` {
		t.Fatalf("got %d %s", w.Code, w.Body)
	}
	got := request(t, h, http.MethodGet, "")
	if got.Code != http.StatusOK || strings.TrimSpace(got.Body.String()) != `{"todos":[]}` {
		t.Fatalf("rejected request changed the list: %d %s", got.Code, got.Body)
	}
}

// REQ-04 #14: 제목 길이는 앞뒤 공백 제거 후 유니코드 코드 포인트로 200자까지 허용한다.
func TestTitleLengthUsesRunes(t *testing.T) {
	h := (&todoStore{}).handler()
	for _, tc := range []struct {
		title  string
		status int
	}{
		{" " + strings.Repeat("가", 200) + " ", http.StatusCreated},
		{strings.Repeat("가", 201), http.StatusBadRequest},
		{strings.Repeat("e\u0301", 100), http.StatusCreated},
		{strings.Repeat("e\u0301", 101), http.StatusBadRequest},
	} {
		body, _ := json.Marshal(map[string]string{"title": tc.title})
		w := request(t, h, http.MethodPost, string(body))
		if w.Code != tc.status {
			t.Fatalf("runes=%d status=%d want %d", len([]rune(strings.TrimSpace(tc.title))), w.Code, tc.status)
		}
	}
}

// REQ-05 #18, #23: 조회 결과는 추가한 순서대로 반환한다.
func TestTodosPreserveCreationOrder(t *testing.T) {
	h := (&todoStore{}).handler()
	for _, title := range []string{"첫째", "둘째", "셋째"} {
		request(t, h, http.MethodPost, `{"title":"`+title+`"}`)
	}
	var got struct {
		Todos []Todo `json:"todos"`
	}
	if err := json.Unmarshal(request(t, h, http.MethodGet, "").Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Todos) != 3 || got.Todos[0].Title != "첫째" || got.Todos[1].Title != "둘째" || got.Todos[2].Title != "셋째" {
		t.Fatalf("unexpected order: %#v", got.Todos)
	}
}

// REQ-06: 새 서버 인스턴스의 할 일 목록은 비어 있다.
func TestNewServerStartsWithEmptyTodoList(t *testing.T) {
	h := (&todoStore{}).handler()
	w := request(t, h, http.MethodGet, "")
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"todos":[]}` {
		t.Fatalf("new server list: status=%d body=%s", w.Code, w.Body)
	}
}

// REQ-07: 빈 목록 안내 문구는 화면 HTML에만 있고 빈 목록 API 응답에는 없다.
func TestEmptyMessageIsOnlyInHomePage(t *testing.T) {
	const message = "아직 할 일이 없으니 위 입력 칸에서 첫 할 일을 추가해 보세요."
	h := (&todoStore{}).handler()
	page := httptest.NewRecorder()
	h.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `id="empty-message"`) || !strings.Contains(page.Body.String(), message) {
		t.Fatalf("home page does not contain the empty-list message: status=%d", page.Code)
	}
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), `<p id="empty-message" hidden>`+message+`</p>`) {
		t.Fatalf("home page does not contain the expected empty message")
	}
	w := request(t, h, http.MethodGet, "")
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"todos":[]}` || strings.Contains(w.Body.String(), message) {
		t.Fatalf("empty list response must contain only an empty todos array: %d %s", w.Code, w.Body)
	}
}

// REQ-08: 안내 문구의 표시 여부는 목록 개수로 갱신되며 추가 성공 뒤 목록을 다시 불러온다.
func TestEmptyMessageVisibilityFollowsTodoCount(t *testing.T) {
	h := (&todoStore{}).handler()
	page := httptest.NewRecorder()
	h.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK {
		t.Fatalf("home page status: %d", page.Code)
	}
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	source := string(html)
	for _, fragment := range []string{
		`<p id="empty-message" hidden>`,
		`emptyMessage.hidden = items.length !== 0;`,
		`await refresh();`,
		`const data = await response.json();`,
		`render(data.todos);`,
	} {
		if !strings.Contains(source, fragment) {
			t.Errorf("home page is missing behavior %q", fragment)
		}
	}

	// The successful create changes the server list; the page's refresh fetches
	// this updated list and render() hides the message when its length is nonzero.
	created := request(t, h, http.MethodPost, `{"title":"회의 자료 준비"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status: %d %s", created.Code, created.Body)
	}
	listed := request(t, h, http.MethodGet, "")
	var got struct {
		Todos []Todo `json:"todos"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if listed.Code != http.StatusOK || len(got.Todos) != 1 {
		t.Fatalf("updated list: status=%d todos=%#v", listed.Code, got.Todos)
	}
	empty := request(t, (&todoStore{}).handler(), http.MethodGet, "")
	if empty.Code != http.StatusOK || strings.TrimSpace(empty.Body.String()) != `{"todos":[]}` {
		t.Fatalf("empty list must remain empty: status=%d body=%s", empty.Code, empty.Body)
	}
}

// REQ-09: 마감일은 선택 항목이며, 넣으면 그대로 돌려주고 없으면 응답에 due 항목을 넣지 않는다.
func TestCreateTodoWithOptionalDue(t *testing.T) {
	h := (&todoStore{}).handler()
	for _, tc := range []struct {
		body string
		want string
	}{
		{`{"title":"회의 자료 준비","due":"2026-10-20"}`, `{"id":1,"title":"회의 자료 준비","due":"2026-10-20"}`},
		{`{"title":"메일 답장"}`, `{"id":2,"title":"메일 답장"}`},
		{`{"title":"메일 답장","due":null}`, `{"id":3,"title":"메일 답장"}`},
		{`{"title":"메일 답장","due":""}`, `{"id":4,"title":"메일 답장"}`},
		{`{"title":"윤년","due":"2028-02-29"}`, `{"id":5,"title":"윤년","due":"2028-02-29"}`},
	} {
		w := request(t, h, http.MethodPost, tc.body)
		if w.Code != http.StatusCreated || strings.TrimSpace(w.Body.String()) != tc.want {
			t.Fatalf("%s: got %d %s, want %s", tc.body, w.Code, w.Body, tc.want)
		}
	}
	for _, body := range []string{`{"title":"메일 답장","due":20261020}`, `{"due":true}`} {
		w := request(t, h, http.MethodPost, body)
		if w.Code != http.StatusBadRequest || strings.TrimSpace(w.Body.String()) != `{"error":"요청 형식이 올바르지 않습니다."}` {
			t.Fatalf("%s: got %d %s", body, w.Code, w.Body)
		}
	}
	var got struct {
		Todos []Todo `json:"todos"`
	}
	if err := json.Unmarshal(request(t, h, http.MethodGet, "").Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Todos) != 5 {
		t.Fatalf("list length = %d, want 5", len(got.Todos))
	}
}

// REQ-10: 목록 조회 응답과 화면은 마감일이 있는 할 일에만 마감일을 함께 보여 준다.
func TestListShowsDueDate(t *testing.T) {
	h := (&todoStore{}).handler()
	request(t, h, http.MethodPost, `{"title":"회의 자료 준비","due":"2026-10-20"}`)
	request(t, h, http.MethodPost, `{"title":"메일 답장"}`)
	w := request(t, h, http.MethodGet, "")
	want := `{"todos":[{"id":1,"title":"회의 자료 준비","due":"2026-10-20"},{"id":2,"title":"메일 답장"}]}`
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != want {
		t.Fatalf("got %d %s, want %s", w.Code, w.Body, want)
	}
	page := httptest.NewRecorder()
	h.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	for _, fragment := range []string{
		`<label for="due">마감일(선택)</label>`,
		`<input type="date" id="due">`,
		"due: dueInput.value",
		"row.textContent = item.due ? `${item.title} · 마감 ${item.due}` : item.title;",
	} {
		if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), fragment) {
			t.Errorf("home page is missing %q (status %d)", fragment, page.Code)
		}
	}
}

// REQ-11: 형식이 틀리거나 달력에 없는 마감일은 거절하고 목록을 바꾸지 않는다.
func TestCreateRejectsInvalidDue(t *testing.T) {
	h := (&todoStore{}).handler()
	for _, due := range []string{
		"2026-02-30", "2027-02-29", "2026-13-01", "2026-04-31", "2026-10-2",
		"2026/10/20", "20261020", " 2026-10-20", "2026-10-20T09:00:00Z",
	} {
		body, _ := json.Marshal(map[string]string{"title": "회의 자료 준비", "due": due})
		w := request(t, h, http.MethodPost, string(body))
		if w.Code != http.StatusBadRequest || strings.TrimSpace(w.Body.String()) != `{"error":"마감일은 실제 있는 날짜를 YYYY-MM-DD 형식으로 입력해 주세요."}` {
			t.Fatalf("due %q: got %d %s", due, w.Code, w.Body)
		}
	}
	w := request(t, h, http.MethodPost, `{"title":"","due":"2026-02-30"}`)
	if w.Code != http.StatusBadRequest || strings.TrimSpace(w.Body.String()) != `{"error":"제목을 입력해 주세요."}` {
		t.Fatalf("blank title with invalid due: got %d %s", w.Code, w.Body)
	}
	if got := request(t, h, http.MethodGet, ""); strings.TrimSpace(got.Body.String()) != `{"todos":[]}` {
		t.Fatalf("list changed: %s", got.Body)
	}
}
