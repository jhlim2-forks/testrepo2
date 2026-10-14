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
	for _, body := range []string{`{"title":""}`, `{}`} {
		w := request(t, h, http.MethodPost, body)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "제목을 입력해 주세요.") {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body)
		}
	}
	if got := request(t, h, http.MethodGet, "").Body.String(); !strings.Contains(got, `"todos":[]`) {
		t.Fatalf("list changed: %s", got)
	}
}

// REQ-03 #35: 공백 문자만 있는 제목은 빈 제목과 같은 메시지로 거절하고 목록을 바꾸지 않는다.
func TestRejectsWhitespaceOnlyTitleIssue35(t *testing.T) {
	h := (&todoStore{}).handler()
	for _, body := range []string{`{"title":"   "}`, `{"title":" \t\n "}`} {
		w := request(t, h, http.MethodPost, body)
		if w.Code != http.StatusBadRequest || strings.TrimSpace(w.Body.String()) != `{"error":"제목을 입력해 주세요."}` {
			t.Fatalf("%s: got %d %s", body, w.Code, w.Body)
		}
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

// REQ-04 #71: 200자를 넘는 제목은 API 계약의 문장으로 거절하고 목록을 바꾸지 않는다.
func TestTooLongTitleUsesContractMessageIssue71(t *testing.T) {
	h := (&todoStore{}).handler()
	for _, title := range []string{strings.Repeat("가", 201), strings.Repeat("é", 101)} {
		body, _ := json.Marshal(map[string]string{"title": title})
		w := request(t, h, http.MethodPost, string(body))
		if w.Code != http.StatusBadRequest || strings.TrimSpace(w.Body.String()) != `{"error":"제목은 200자까지 입력할 수 있습니다."}` {
			t.Fatalf("runes=%d: got %d %s", len([]rune(title)), w.Code, w.Body)
		}
	}
	got := request(t, h, http.MethodGet, "")
	if got.Code != http.StatusOK || strings.TrimSpace(got.Body.String()) != `{"todos":[]}` {
		t.Fatalf("rejected request changed the list: %d %s", got.Code, got.Body)
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

func requestPath(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// REQ-12: 할 일마다 완료 표시를 하거나 풀 수 있고, 새 할 일은 완료하지 않은 상태로 시작한다.
func TestToggleDone(t *testing.T) {
	h := (&todoStore{}).handler()
	if w := request(t, h, http.MethodPost, `{"title":"회의 자료 준비","done":true}`); w.Code != http.StatusCreated || strings.TrimSpace(w.Body.String()) != `{"id":1,"title":"회의 자료 준비"}` {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	request(t, h, http.MethodPost, `{"title":"메일 답장","due":"2026-10-20"}`)
	if got := strings.TrimSpace(request(t, h, http.MethodGet, "").Body.String()); got != `{"todos":[{"id":1,"title":"회의 자료 준비"},{"id":2,"title":"메일 답장","due":"2026-10-20"}]}` {
		t.Fatalf("new todos must not be done: %s", got)
	}
	for i := 0; i < 2; i++ {
		w := requestPath(t, h, http.MethodPatch, "/api/todos/1", `{"done":true}`)
		if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"id":1,"title":"회의 자료 준비","done":true}` {
			t.Fatalf("mark done #%d: %d %s", i+1, w.Code, w.Body)
		}
	}
	if got := request(t, h, http.MethodGet, "").Body.String(); !strings.Contains(got, `{"id":1,"title":"회의 자료 준비","done":true}`) {
		t.Fatalf("list must keep done: %s", got)
	}
	w := requestPath(t, h, http.MethodPatch, "/api/todos/1", `{"done":false}`)
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"id":1,"title":"회의 자료 준비"}` {
		t.Fatalf("unmark: %d %s", w.Code, w.Body)
	}
	if got := request(t, h, http.MethodGet, "").Body.String(); strings.Contains(got, `"done"`) {
		t.Fatalf("list must not contain done: %s", got)
	}
	w = requestPath(t, h, http.MethodPatch, "/api/todos/2", `{"done":true,"due":"2027-01-01"}`)
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"id":2,"title":"메일 답장","due":"2026-10-20","done":true}` {
		t.Fatalf("title and due must not change: %d %s", w.Code, w.Body)
	}
}

// REQ-13: 목록 조회는 완료하지 않은 할 일을 먼저, 완료한 할 일을 뒤에 두고 각 묶음 안은 추가한 순서를 따른다.
func TestListPutsDoneTodosLast(t *testing.T) {
	h := (&todoStore{}).handler()
	for _, title := range []string{"가", "나", "다"} {
		request(t, h, http.MethodPost, `{"title":"`+title+`"}`)
	}
	list := func() string {
		return strings.TrimSpace(request(t, h, http.MethodGet, "").Body.String())
	}
	if got := list(); got != `{"todos":[{"id":1,"title":"가"},{"id":2,"title":"나"},{"id":3,"title":"다"}]}` {
		t.Fatalf("no done todos: %s", got)
	}
	requestPath(t, h, http.MethodPatch, "/api/todos/1", `{"done":true}`)
	if got := list(); got != `{"todos":[{"id":2,"title":"나"},{"id":3,"title":"다"},{"id":1,"title":"가","done":true}]}` {
		t.Fatalf("after marking 1: %s", got)
	}
	requestPath(t, h, http.MethodPatch, "/api/todos/2", `{"done":true}`)
	if got := list(); got != `{"todos":[{"id":3,"title":"다"},{"id":1,"title":"가","done":true},{"id":2,"title":"나","done":true}]}` {
		t.Fatalf("after marking 2: %s", got)
	}
	requestPath(t, h, http.MethodPatch, "/api/todos/1", `{"done":false}`)
	if got := list(); got != `{"todos":[{"id":1,"title":"가"},{"id":3,"title":"다"},{"id":2,"title":"나","done":true}]}` {
		t.Fatalf("after unmarking 1: %s", got)
	}
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"checkbox.type = 'checkbox';",
		"method: 'PATCH'",
		"body: JSON.stringify({ done })",
		"await fetch(`/api/todos/${id}`",
	} {
		if !strings.Contains(string(html), fragment) {
			t.Errorf("home page is missing behavior %q", fragment)
		}
	}
}

// REQ-14: 화면은 완료한 할 일 줄을 done 클래스로 흐리게 보여 주고 숨기지 않으며, 안내 문구는 전체 할 일 수로만 정한다.
func TestDoneTodosAreDimmedNotHidden(t *testing.T) {
	h := (&todoStore{}).handler()
	page := httptest.NewRecorder()
	h.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK {
		t.Fatalf("home page status: %d", page.Code)
	}
	for _, fragment := range []string{
		`li.done { opacity: .5; }`,
		`if (item.done) row.classList.add('done');`,
		`checkbox.checked = item.done === true;`,
		`'aria-label', '완료'`,
		`emptyMessage.hidden = items.length !== 0;`,
	} {
		if !strings.Contains(page.Body.String(), fragment) {
			t.Errorf("home page is missing %q", fragment)
		}
	}
	request(t, h, http.MethodPost, `{"title":"회의 자료 준비"}`)
	requestPath(t, h, http.MethodPatch, "/api/todos/1", `{"done":true}`)
	if got := strings.TrimSpace(request(t, h, http.MethodGet, "").Body.String()); got != `{"todos":[{"id":1,"title":"회의 자료 준비","done":true}]}` {
		t.Fatalf("done todo must stay in the list: %s", got)
	}
}

// REQ-15: 없는 할 일은 404, 잘못된 요청 형식은 400으로 거절하고 완료 여부를 바꾸지 않는다.
func TestToggleDoneRejectsInvalidRequests(t *testing.T) {
	h := (&todoStore{}).handler()
	request(t, h, http.MethodPost, `{"title":"회의 자료 준비"}`)
	const notFound = `{"error":"할 일을 찾을 수 없습니다."}`
	for _, path := range []string{"/api/todos/99", "/api/todos/0", "/api/todos/abc", "/api/todos/-1", "/api/todos/", "/api/todos/1/x"} {
		w := requestPath(t, h, http.MethodPatch, path, `{"done":true}`)
		if w.Code != http.StatusNotFound || strings.TrimSpace(w.Body.String()) != notFound {
			t.Fatalf("%s: got %d %s", path, w.Code, w.Body)
		}
	}
	if w := requestPath(t, h, http.MethodPatch, "/api/todos/99", `{"done":`); w.Code != http.StatusNotFound || strings.TrimSpace(w.Body.String()) != notFound {
		t.Fatalf("missing id with bad body: got %d %s", w.Code, w.Body)
	}
	for _, done := range []bool{false, true} {
		if done {
			requestPath(t, h, http.MethodPatch, "/api/todos/1", `{"done":true}`)
		}
		want := request(t, h, http.MethodGet, "").Body.String()
		for _, body := range []string{`{"done":`, ``, `{}`, `{"done":null}`, `{"done":"true"}`, `{"done":1}`, `[]`, `{"done":true} {}`} {
			w := requestPath(t, h, http.MethodPatch, "/api/todos/1", body)
			if w.Code != http.StatusBadRequest || strings.TrimSpace(w.Body.String()) != `{"error":"요청 형식이 올바르지 않습니다."}` {
				t.Fatalf("%q: got %d %s", body, w.Code, w.Body)
			}
			if got := request(t, h, http.MethodGet, "").Body.String(); got != want {
				t.Fatalf("%q changed the list: %s, want %s", body, got, want)
			}
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut} {
		w := requestPath(t, h, method, "/api/todos/1", `{"done":false}`)
		if w.Code != http.StatusMethodNotAllowed || strings.TrimSpace(w.Body.String()) != `{"error":"지원하지 않는 요청 방식입니다."}` {
			t.Fatalf("%s: got %d %s", method, w.Code, w.Body)
		}
	}
	if w := requestPath(t, h, http.MethodDelete, "/api/todos", ""); w.Code != http.StatusMethodNotAllowed || strings.TrimSpace(w.Body.String()) != `{"error":"지원하지 않는 요청 방식입니다."}` {
		t.Fatalf("DELETE /api/todos: got %d %s", w.Code, w.Body)
	}
	if w := requestPath(t, h, http.MethodPatch, "/api/todos", `{"done":true}`); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PATCH /api/todos: got %d %s", w.Code, w.Body)
	}
}

// REQ-16: 제목을 고치면 고친 제목을 보관하고, 식별자, 마감일, 완료 여부와 목록 순서는 그대로 둔다.
func TestEditTitle(t *testing.T) {
	h := (&todoStore{}).handler()
	request(t, h, http.MethodPost, `{"title":"회의 자료 준비","due":"2026-10-20"}`)
	request(t, h, http.MethodPost, `{"title":"메일 답장"}`)
	requestPath(t, h, http.MethodPatch, "/api/todos/2", `{"done":true}`)
	w := requestPath(t, h, http.MethodPatch, "/api/todos/1", `{"title":"  회의 자료 다시 준비  "}`)
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"id":1,"title":"회의 자료 다시 준비","due":"2026-10-20"}` {
		t.Fatalf("edit 1: %d %s", w.Code, w.Body)
	}
	list := func() string {
		return strings.TrimSpace(request(t, h, http.MethodGet, "").Body.String())
	}
	if got := list(); got != `{"todos":[{"id":1,"title":"회의 자료 다시 준비","due":"2026-10-20"},{"id":2,"title":"메일 답장","done":true}]}` {
		t.Fatalf("list after edit 1: %s", got)
	}
	w = requestPath(t, h, http.MethodPatch, "/api/todos/2", `{"title":"메일 다시 답장"}`)
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"id":2,"title":"메일 다시 답장","done":true}` {
		t.Fatalf("edit 2 must keep done: %d %s", w.Code, w.Body)
	}
	w = requestPath(t, h, http.MethodPatch, "/api/todos/2", `{"title":"메일 다시 답장"}`)
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"id":2,"title":"메일 다시 답장","done":true}` {
		t.Fatalf("same title: %d %s", w.Code, w.Body)
	}
	w = requestPath(t, h, http.MethodPatch, "/api/todos/2", `{"done":false,"title":"메일 답장"}`)
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"id":2,"title":"메일 답장"}` {
		t.Fatalf("done and title together: %d %s", w.Code, w.Body)
	}
	w = requestPath(t, h, http.MethodPatch, "/api/todos/1", `{"title":"회의 자료 준비","due":"2027-01-01","id":5}`)
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"id":1,"title":"회의 자료 준비","due":"2026-10-20"}` {
		t.Fatalf("due and id must not change: %d %s", w.Code, w.Body)
	}
	if got := list(); got != `{"todos":[{"id":1,"title":"회의 자료 준비","due":"2026-10-20"},{"id":2,"title":"메일 답장"}]}` {
		t.Fatalf("final list: %s", got)
	}
	page := httptest.NewRecorder()
	h.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	for _, fragment := range []string{
		`editButton.textContent = '수정';`,
		`saveButton.textContent = '저장';`,
		`cancelButton.textContent = '취소';`,
		`body: JSON.stringify({ title })`,
		`titleText.textContent = item.title;`,
		`event.key === 'Enter'`,
		`event.key === 'Escape'`,
	} {
		if !strings.Contains(page.Body.String(), fragment) {
			t.Errorf("home page is missing %q", fragment)
		}
	}
}

// REQ-17: 고칠 제목에는 추가와 같은 제목 규칙과 오류 문장을 적용하고, 거절하면 제목과 완료 여부를 그대로 둔다.
func TestEditTitleUsesCreateTitleRules(t *testing.T) {
	h := (&todoStore{}).handler()
	request(t, h, http.MethodPost, `{"title":"회의 자료 준비"}`)
	const unchanged = `{"todos":[{"id":1,"title":"회의 자료 준비"}]}`
	cases := []struct{ body, want string }{
		{`{"title":""}`, `{"error":"제목을 입력해 주세요."}`},
		{`{"title":"   "}`, `{"error":"제목을 입력해 주세요."}`},
		{`{"title":" \t\n "}`, `{"error":"제목을 입력해 주세요."}`},
		{`{"title":"` + strings.Repeat("가", 201) + `"}`, `{"error":"제목은 200자까지 입력할 수 있습니다."}`},
		{`{"title":"` + strings.Repeat("e\u0301", 101) + `"}`, `{"error":"제목은 200자까지 입력할 수 있습니다."}`},
		{`{"done":true,"title":""}`, `{"error":"제목을 입력해 주세요."}`},
	}
	for _, c := range cases {
		w := requestPath(t, h, http.MethodPatch, "/api/todos/1", c.body)
		if w.Code != http.StatusBadRequest || strings.TrimSpace(w.Body.String()) != c.want {
			t.Fatalf("%.40s: got %d %s", c.body, w.Code, w.Body)
		}
		if got := strings.TrimSpace(request(t, h, http.MethodGet, "").Body.String()); got != unchanged {
			t.Fatalf("%.40s changed the list: %s", c.body, got)
		}
		// 추가할 때도 같은 문장으로 거절한다.
		if w := request(t, h, http.MethodPost, c.body); strings.TrimSpace(w.Body.String()) != c.want {
			t.Fatalf("%.40s: create and edit disagree: %s", c.body, w.Body)
		}
	}
	for _, c := range []struct{ body, want string }{
		{`{"title":"` + strings.Repeat("가", 200) + `"}`, strings.Repeat("가", 200)},
		{`{"title":" ` + strings.Repeat("나", 200) + ` "}`, strings.Repeat("나", 200)},
		{`{"title":"` + strings.Repeat("e\u0301", 100) + `"}`, strings.Repeat("e\u0301", 100)},
	} {
		w := requestPath(t, h, http.MethodPatch, "/api/todos/1", c.body)
		var got Todo
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if w.Code != http.StatusOK || got.Title != c.want || len([]rune(got.Title)) != 200 {
			t.Fatalf("%.40s: got %d %q", c.body, w.Code, got.Title)
		}
	}
}

// REQ-18: 없는 할 일은 404, 잘못된 형식은 400으로 거절하고 제목과 완료 여부를 바꾸지 않는다.
func TestEditTitleRejectsInvalidRequests(t *testing.T) {
	h := (&todoStore{}).handler()
	request(t, h, http.MethodPost, `{"title":"회의 자료 준비"}`)
	const notFound = `{"error":"할 일을 찾을 수 없습니다."}`
	for _, path := range []string{"/api/todos/99", "/api/todos/0", "/api/todos/abc"} {
		for _, body := range []string{`{"title":"새 제목"}`, `{"title":""}`} {
			w := requestPath(t, h, http.MethodPatch, path, body)
			if w.Code != http.StatusNotFound || strings.TrimSpace(w.Body.String()) != notFound {
				t.Fatalf("%s %s: got %d %s", path, body, w.Code, w.Body)
			}
		}
	}
	for _, done := range []bool{false, true} {
		if done {
			requestPath(t, h, http.MethodPatch, "/api/todos/1", `{"done":true}`)
		}
		want := request(t, h, http.MethodGet, "").Body.String()
		for _, body := range []string{`{"title":null}`, `{"title":123}`, `{"done":true,"title":123}`, `{"done":"true","title":"새 제목"}`, `{"done":1,"title":""}`, `{"done":false,"title":null}`} {
			w := requestPath(t, h, http.MethodPatch, "/api/todos/1", body)
			if w.Code != http.StatusBadRequest || strings.TrimSpace(w.Body.String()) != `{"error":"요청 형식이 올바르지 않습니다."}` {
				t.Fatalf("%s: got %d %s", body, w.Code, w.Body)
			}
			if got := request(t, h, http.MethodGet, "").Body.String(); got != want {
				t.Fatalf("%s changed the list: %s, want %s", body, got, want)
			}
		}
	}
}

// REQ-19: 화면은 목록 영역 바로 위에 완료하지 않은 할 일 개수를 보여 주고, 할 일이 없으면 숨기며, API 응답 형식은 바꾸지 않는다.
func TestRemainingCountIsShownAboveList(t *testing.T) {
	h := (&todoStore{}).handler()
	page := httptest.NewRecorder()
	h.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK {
		t.Fatalf("home page status: %d", page.Code)
	}
	source := page.Body.String()
	countAt := strings.Index(source, `<p id="remaining-count" hidden></p>`)
	listAt := strings.Index(source, `<ul id="todos">`)
	if countAt < 0 || listAt < 0 || countAt > listAt {
		t.Fatalf("remaining count must be placed before the list: count=%d list=%d", countAt, listAt)
	}
	if formEnd := strings.Index(source, `</form>`); formEnd < 0 || formEnd > countAt {
		t.Fatalf("remaining count must be placed after the add form")
	}
	for _, fragment := range []string{
		"remainingCount.textContent = `남은 할 일 ${items.filter(item => item.done !== true).length}개`;",
		`remainingCount.hidden = items.length === 0;`,
		`emptyMessage.hidden = items.length !== 0;`,
	} {
		if !strings.Contains(source, fragment) {
			t.Errorf("home page is missing behavior %q", fragment)
		}
	}

	empty := request(t, h, http.MethodGet, "")
	if empty.Code != http.StatusOK || strings.TrimSpace(empty.Body.String()) != `{"todos":[]}` || strings.Contains(empty.Body.String(), "남은 할 일") {
		t.Fatalf("empty list response must not contain the remaining count: %d %s", empty.Code, empty.Body)
	}

	// The page counts todos whose done is not true from this list response.
	for _, title := range []string{"가", "나", "다"} {
		if w := request(t, h, http.MethodPost, `{"title":"`+title+`"}`); w.Code != http.StatusCreated {
			t.Fatalf("create %s: %d %s", title, w.Code, w.Body)
		}
	}
	remaining := func() int {
		t.Helper()
		var got struct {
			Todos []Todo `json:"todos"`
		}
		w := request(t, h, http.MethodGet, "")
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(w.Body.String(), "남은 할 일") {
			t.Fatalf("list response must not contain the remaining count: %s", w.Body)
		}
		n := 0
		for _, todo := range got.Todos {
			if !todo.Done {
				n++
			}
		}
		return n
	}
	if n := remaining(); n != 3 {
		t.Fatalf("remaining after three creates: %d", n)
	}
	requestPath(t, h, http.MethodPatch, "/api/todos/1", `{"done":true}`)
	if n := remaining(); n != 2 {
		t.Fatalf("remaining after marking one done: %d", n)
	}
	requestPath(t, h, http.MethodPatch, "/api/todos/1", `{"done":false}`)
	if n := remaining(); n != 3 {
		t.Fatalf("remaining after unmarking: %d", n)
	}
	request(t, h, http.MethodPost, `{"title":"  "}`)
	if n := remaining(); n != 3 {
		t.Fatalf("remaining after rejected create: %d", n)
	}
	for _, id := range []string{"1", "2", "3"} {
		requestPath(t, h, http.MethodPatch, "/api/todos/"+id, `{"done":true}`)
	}
	if n := remaining(); n != 0 {
		t.Fatalf("remaining after completing all: %d", n)
	}
}

// REQ-20: 화면은 남은 할 일 개수 뒤에 같은 요소로 완료한 할 일 개수를 이어 보여 주고, API 응답에는 개수를 담지 않는다.
func TestCompletedCountIsShownWithRemainingCount(t *testing.T) {
	h := (&todoStore{}).handler()
	page := httptest.NewRecorder()
	h.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK {
		t.Fatalf("home page status: %d", page.Code)
	}
	source := page.Body.String()
	if n := strings.Count(source, `id="remaining-count"`); n != 1 {
		t.Fatalf("counts must share one remaining-count element: %d", n)
	}
	if strings.Contains(source, `id="completed-count"`) {
		t.Fatalf("completed count must not have its own element")
	}
	remainingLine := "remainingCount.textContent = `남은 할 일 ${items.filter(item => item.done !== true).length}개`;"
	completedLine := "remainingCount.textContent += ` · 완료 ${items.filter(item => item.done === true).length}개`;"
	remainingAt := strings.Index(source, remainingLine)
	completedAt := strings.Index(source, completedLine)
	if remainingAt < 0 || completedAt < 0 || completedAt < remainingAt {
		t.Fatalf("home page must append the completed count after the remaining count: remaining=%d completed=%d", remainingAt, completedAt)
	}
	if !strings.Contains(source, `remainingCount.hidden = items.length === 0;`) {
		t.Fatalf("count line must be hidden when there are no todos")
	}

	// The page counts done and not-done todos from the same list response.
	counts := func() (remaining, completed int) {
		t.Helper()
		var got map[string][]Todo
		w := request(t, h, http.MethodGet, "")
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got["todos"] == nil {
			t.Fatalf("list response format must not change: %s", w.Body)
		}
		if strings.Contains(w.Body.String(), "완료") || strings.Contains(w.Body.String(), "남은 할 일") {
			t.Fatalf("list response must not contain counts: %s", w.Body)
		}
		for _, todo := range got["todos"] {
			if todo.Done {
				completed++
			} else {
				remaining++
			}
		}
		if remaining+completed != len(got["todos"]) {
			t.Fatalf("counts must add up to the number of todos")
		}
		return remaining, completed
	}
	if r, c := counts(); r != 0 || c != 0 {
		t.Fatalf("empty list counts: %d %d", r, c)
	}
	for _, title := range []string{"가", "나", "다"} {
		if w := request(t, h, http.MethodPost, `{"title":"`+title+`"}`); w.Code != http.StatusCreated {
			t.Fatalf("create %s: %d %s", title, w.Code, w.Body)
		}
	}
	if r, c := counts(); r != 3 || c != 0 {
		t.Fatalf("counts after three creates: %d %d", r, c)
	}
	requestPath(t, h, http.MethodPatch, "/api/todos/1", `{"done":true}`)
	if r, c := counts(); r != 2 || c != 1 {
		t.Fatalf("counts after marking one done: %d %d", r, c)
	}
	requestPath(t, h, http.MethodPatch, "/api/todos/1", `{"done":false}`)
	if r, c := counts(); r != 3 || c != 0 {
		t.Fatalf("counts after unmarking: %d %d", r, c)
	}
	for _, id := range []string{"1", "2", "3"} {
		requestPath(t, h, http.MethodPatch, "/api/todos/"+id, `{"done":true}`)
	}
	if r, c := counts(); r != 0 || c != 3 {
		t.Fatalf("counts after completing all: %d %d", r, c)
	}
}

// REQ-22: 화면은 목록 위의 검색 칸으로 받아 온 목록을 제목으로 거르고, 결과가 없으면 따로 알리며, 서버는 검색어를 받지 않는다.
func TestSearchFiltersOnPageOnly(t *testing.T) {
	h := (&todoStore{}).handler()
	page := httptest.NewRecorder()
	h.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK {
		t.Fatalf("home page status: %d", page.Code)
	}
	source := page.Body.String()
	searchAt := strings.Index(source, `<input type="search" id="search" placeholder="제목으로 찾기">`)
	labelAt := strings.Index(source, `<label for="search">제목으로 찾기</label>`)
	countAt := strings.Index(source, `<p id="remaining-count" hidden></p>`)
	listAt := strings.Index(source, `<ul id="todos">`)
	formEnd := strings.Index(source, `</form>`)
	if searchAt < 0 || labelAt < 0 || countAt < 0 || listAt < 0 || formEnd < 0 {
		t.Fatalf("home page is missing the search box, its label, the count line or the list")
	}
	if !(formEnd < labelAt && labelAt < searchAt && searchAt < countAt && countAt < listAt) {
		t.Fatalf("search box must be between the add form and the count line: form=%d label=%d search=%d count=%d list=%d", formEnd, labelAt, searchAt, countAt, listAt)
	}
	noResultsAt := strings.Index(source, `<p id="no-results" hidden>찾는 할 일이 없습니다.</p>`)
	if noResultsAt < listAt {
		t.Fatalf("no-results message must be a hidden element after the list: %d", noResultsAt)
	}
	for _, fragment := range []string{
		`return query === '' || item.title.toLowerCase().includes(query.toLowerCase());`,
		`const query = search.value.trim();`,
		`const shown = items.filter(item => matches(item, query));`,
		`noResults.hidden = !(items.length !== 0 && shown.length === 0);`,
		`for (const item of shown) {`,
		`emptyMessage.hidden = items.length !== 0;`,
		`remainingCount.hidden = items.length === 0;`,
		`search.addEventListener('input', () => render(lastItems));`,
		`const response = await fetch('/api/todos');`,
	} {
		if !strings.Contains(source, fragment) {
			t.Errorf("home page is missing behavior %q", fragment)
		}
	}

	// The server ignores any search query and always returns the full list.
	for _, title := range []string{"회의 자료 준비", "메일 답장", "Weekly Report"} {
		if w := request(t, h, http.MethodPost, `{"title":"`+title+`"}`); w.Code != http.StatusCreated {
			t.Fatalf("create %s: %d %s", title, w.Code, w.Body)
		}
	}
	requestPath(t, h, http.MethodPatch, "/api/todos/2", `{"done":true}`)
	full := request(t, h, http.MethodGet, "")
	queried := requestPath(t, h, http.MethodGet, "/api/todos?q=회의", "")
	if queried.Code != http.StatusOK || queried.Body.String() != full.Body.String() {
		t.Fatalf("query must not change the list response: %d %s, want %s", queried.Code, queried.Body, full.Body)
	}
	if strings.Contains(queried.Body.String(), "찾는 할 일이 없습니다.") {
		t.Fatalf("list response must not contain the no-results message: %s", queried.Body)
	}
	want := `{"todos":[{"id":1,"title":"회의 자료 준비"},{"id":3,"title":"Weekly Report"},{"id":2,"title":"메일 답장","done":true}]}`
	if got := strings.TrimSpace(queried.Body.String()); got != want {
		t.Fatalf("list order: %s", got)
	}
}

// REQ-23: 할 일을 지우면 본문 없이 204를 돌려주고, 남은 할 일은 그대로 두며, 지운 식별자는 다시 쓰지 않는다.
func TestDeleteTodo(t *testing.T) {
	h := (&todoStore{}).handler()
	request(t, h, http.MethodPost, `{"title":"회의 자료 준비","due":"2026-10-20"}`)
	request(t, h, http.MethodPost, `{"title":"메일 답장"}`)
	request(t, h, http.MethodPost, `{"title":"보고서 제출"}`)
	requestPath(t, h, http.MethodPatch, "/api/todos/2", `{"done":true}`)
	steps := []struct{ id, body, list string }{
		{"2", "", `{"todos":[{"id":1,"title":"회의 자료 준비","due":"2026-10-20"},{"id":3,"title":"보고서 제출"}]}`},
		{"1", `{"id":3}`, `{"todos":[{"id":3,"title":"보고서 제출"}]}`},
		{"3", `{"id":1}`, `{"todos":[]}`},
	}
	for _, step := range steps {
		w := requestPath(t, h, http.MethodDelete, "/api/todos/"+step.id, step.body)
		if w.Code != http.StatusNoContent || w.Body.Len() != 0 {
			t.Fatalf("delete %s: %d %s", step.id, w.Code, w.Body)
		}
		if got := strings.TrimSpace(request(t, h, http.MethodGet, "").Body.String()); got != step.list {
			t.Fatalf("list after deleting %s: %s, want %s", step.id, got, step.list)
		}
	}
	if w := request(t, h, http.MethodPost, `{"title":"가"}`); w.Code != http.StatusCreated || strings.TrimSpace(w.Body.String()) != `{"id":4,"title":"가"}` {
		t.Fatalf("deleted identifiers must not be reused: %d %s", w.Code, w.Body)
	}

	// Deleting from the middle keeps the remaining todos and their order, and a later todo can still be changed.
	h = (&todoStore{}).handler()
	for _, title := range []string{"가", "나", "다"} {
		request(t, h, http.MethodPost, `{"title":"`+title+`"}`)
	}
	requestPath(t, h, http.MethodDelete, "/api/todos/2", "")
	if w := requestPath(t, h, http.MethodPatch, "/api/todos/3", `{"done":true,"title":"다다"}`); w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"id":3,"title":"다다","done":true}` {
		t.Fatalf("patch after delete: %d %s", w.Code, w.Body)
	}
	if got := strings.TrimSpace(request(t, h, http.MethodGet, "").Body.String()); got != `{"todos":[{"id":1,"title":"가"},{"id":3,"title":"다다","done":true}]}` {
		t.Fatalf("list after delete and patch: %s", got)
	}

	page := httptest.NewRecorder()
	h.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	source := page.Body.String()
	for _, fragment := range []string{
		`deleteButton.textContent = '삭제';`,
		"await fetch(`/api/todos/${id}`, { method: 'DELETE' });",
	} {
		if !strings.Contains(source, fragment) {
			t.Errorf("home page is missing %q", fragment)
		}
	}
}

// REQ-24: 없는 할 일을 지우려 하면 404와 오류 문장을 돌려주고 목록을 바꾸지 않는다.
func TestDeleteRejectsMissingTodo(t *testing.T) {
	h := (&todoStore{}).handler()
	request(t, h, http.MethodPost, `{"title":"회의 자료 준비"}`)
	const notFound = `{"error":"할 일을 찾을 수 없습니다."}`
	want := request(t, h, http.MethodGet, "").Body.String()
	for _, path := range []string{"/api/todos/99", "/api/todos/0", "/api/todos/-1", "/api/todos/abc", "/api/todos/", "/api/todos/1/x"} {
		w := requestPath(t, h, http.MethodDelete, path, "")
		if w.Code != http.StatusNotFound || strings.TrimSpace(w.Body.String()) != notFound {
			t.Fatalf("%s: got %d %s", path, w.Code, w.Body)
		}
		if got := request(t, h, http.MethodGet, "").Body.String(); got != want {
			t.Fatalf("%s changed the list: %s, want %s", path, got, want)
		}
	}
	if w := requestPath(t, h, http.MethodDelete, "/api/todos/1", ""); w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("first delete: %d %s", w.Code, w.Body)
	}
	if w := requestPath(t, h, http.MethodDelete, "/api/todos/1", ""); w.Code != http.StatusNotFound || strings.TrimSpace(w.Body.String()) != notFound {
		t.Fatalf("second delete: %d %s", w.Code, w.Body)
	}
	if w := requestPath(t, h, http.MethodPatch, "/api/todos/1", `{"done":true}`); w.Code != http.StatusNotFound || strings.TrimSpace(w.Body.String()) != notFound {
		t.Fatalf("patch deleted todo: %d %s", w.Code, w.Body)
	}
}

// REQ-23, REQ-24 #95: 할 일 지우기가 성공하면 200과 지운 할 일이 아니라 본문 없는 204를 돌려준다.
func TestDeleteRespondsNoContentIssue95(t *testing.T) {
	h := (&todoStore{}).handler()
	request(t, h, http.MethodPost, `{"title":"회의 자료 준비","due":"2026-10-20"}`)
	request(t, h, http.MethodPost, `{"title":"메일 답장"}`)
	request(t, h, http.MethodPost, `{"title":"보고서 제출"}`)
	requestPath(t, h, http.MethodPatch, "/api/todos/2", `{"done":true}`)
	w := requestPath(t, h, http.MethodDelete, "/api/todos/2", "")
	if w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("delete: got %d %q, want 204 with an empty body", w.Code, w.Body)
	}
	if got := strings.TrimSpace(request(t, h, http.MethodGet, "").Body.String()); got != `{"todos":[{"id":1,"title":"회의 자료 준비","due":"2026-10-20"},{"id":3,"title":"보고서 제출"}]}` {
		t.Fatalf("list after delete: %s", got)
	}
	if w := requestPath(t, h, http.MethodDelete, "/api/todos/2", ""); w.Code != http.StatusNotFound || strings.TrimSpace(w.Body.String()) != `{"error":"할 일을 찾을 수 없습니다."}` {
		t.Fatalf("second delete: %d %s", w.Code, w.Body)
	}
}

// REQ-25: 화면은 지운 뒤 성공과 실패 모두 목록을 다시 받아 같은 그리기 함수로 목록, 안내 문구, 개수 줄을 갱신한다.
func TestDeleteRefreshesListAndCounts(t *testing.T) {
	h := (&todoStore{}).handler()
	page := httptest.NewRecorder()
	h.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	source := page.Body.String()
	start := strings.Index(source, "async function deleteTodo(id) {")
	if start < 0 {
		t.Fatalf("home page must define deleteTodo")
	}
	body := source[start:]
	body = body[:strings.Index(body, "\n    }\n")]
	for _, fragment := range []string{
		"if (closeEdit) closeEdit();",
		"error.textContent = '';",
		"error.textContent = data.error;",
		"await refresh();",
	} {
		if !strings.Contains(body, fragment) {
			t.Errorf("deleteTodo is missing %q", fragment)
		}
	}
	if strings.Contains(body, "confirm(") {
		t.Errorf("deleteTodo must not ask for confirmation")
	}
	if strings.Index(body, "await refresh();") < strings.Index(body, "error.textContent = data.error;") || strings.Contains(body, "return;") {
		t.Errorf("deleteTodo must refresh the list after both success and failure")
	}
	for _, fragment := range []string{
		`emptyMessage.hidden = items.length !== 0;`,
		`remainingCount.hidden = items.length === 0;`,
	} {
		if !strings.Contains(source, fragment) {
			t.Errorf("home page is missing behavior %q", fragment)
		}
	}

	// The list response the page redraws from shrinks with each delete, down to an empty list.
	for _, title := range []string{"가", "나", "다"} {
		request(t, h, http.MethodPost, `{"title":"`+title+`"}`)
	}
	requestPath(t, h, http.MethodPatch, "/api/todos/1", `{"done":true}`)
	steps := []struct{ id, list string }{
		{"2", `{"todos":[{"id":3,"title":"다"},{"id":1,"title":"가","done":true}]}`},
		{"1", `{"todos":[{"id":3,"title":"다"}]}`},
		{"3", `{"todos":[]}`},
	}
	for _, step := range steps {
		requestPath(t, h, http.MethodDelete, "/api/todos/"+step.id, "")
		if got := strings.TrimSpace(request(t, h, http.MethodGet, "").Body.String()); got != step.list {
			t.Fatalf("list after deleting %s: %s, want %s", step.id, got, step.list)
		}
	}
}
