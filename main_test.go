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

// REQ-07: 빈 목록 안내 문구는 화면 HTML에 있고 빈 목록 API 응답에는 없다.
func TestEmptyMessageIsOnlyInHomePage(t *testing.T) {
	h := (&todoStore{}).handler()
	message := "아직 할 일이 없으니 위 입력 칸에서 첫 할 일을 추가해 보세요."

	home := httptest.NewRecorder()
	h.ServeHTTP(home, httptest.NewRequest(http.MethodGet, "/", nil))
	if home.Code != http.StatusOK || !strings.Contains(home.Body.String(), `id="empty-message"`) || !strings.Contains(home.Body.String(), message) {
		t.Fatalf("home page does not contain the empty message element: status=%d body=%s", home.Code, home.Body)
	}

	list := request(t, h, http.MethodGet, "")
	if list.Code != http.StatusOK || strings.TrimSpace(list.Body.String()) != `{"todos":[]}` || strings.Contains(list.Body.String(), message) {
		t.Fatalf("empty list response unexpected: status=%d body=%s", list.Code, list.Body)
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

// REQ-05: 조회 결과는 추가한 순서대로 반환한다.
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

// REQ-08: 렌더링은 항목 수에 따라 전용 안내 문구를 보이고 숨긴다.
func TestEmptyMessageVisibilityTracksRenderedTodos(t *testing.T) {
	html, err := os.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	source := string(html)
	if !strings.Contains(source, `<p id="empty-message" hidden>`) || !strings.Contains(source, "emptyMessage.hidden = items.length !== 0;") {
		t.Fatal("empty message must start hidden and render based on the current todo count")
	}
}
