package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// REQ-04 #12: 제목이 200 코드 포인트 이하면 한글 제목도 생성되어야 한다.
func TestIssue12Accepts200KoreanCodePoints(t *testing.T) {
	h := (&todoStore{}).handler()
	title := strings.Repeat("가", 200)
	body, err := json.Marshal(map[string]string{"title": title})
	if err != nil {
		t.Fatal(err)
	}
	w := request(t, h, http.MethodPost, string(body))
	if w.Code != http.StatusCreated {
		t.Fatalf("got status %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body)
	}
	var got Todo
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID == 0 || got.Title != title {
		t.Fatalf("got todo %#v, want created todo with the original title", got)
	}
}
