package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
)

type Todo struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

type todoStore struct {
	mu    sync.Mutex
	items []Todo
}

func (s *todoStore) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "index.html")
	})
	mux.HandleFunc("/api/todos", s.todos)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *todoStore) todos(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.mu.Lock()
		items := append([]Todo{}, s.items...)
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"todos": items})
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "지원하지 않는 요청 방식입니다."})
		return
	}
	var input map[string]json.RawMessage
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "요청 형식이 올바르지 않습니다."})
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "요청 형식이 올바르지 않습니다."})
		return
	}
	raw, exists := input["title"]
	if !exists {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "제목을 입력해 주세요."})
		return
	}
	var decodedTitle any
	if err := json.Unmarshal(raw, &decodedTitle); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "요청 형식이 올바르지 않습니다."})
		return
	}
	title, ok := decodedTitle.(string)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "요청 형식이 올바르지 않습니다."})
		return
	}
	title = strings.TrimSpace(title)
	if title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "제목을 입력해 주세요."})
		return
	}
	if len([]rune(title)) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "제목은 200자까지 입력할 수 있습니다."})
		return
	}
	s.mu.Lock()
	item := Todo{ID: len(s.items) + 1, Title: title}
	s.items = append(s.items, item)
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, item)
}

func main() {
	listen := flag.String("listen", "127.0.0.1:8091", "HTTP listen address")
	flag.Parse()
	log.Fatal(http.ListenAndServe(*listen, (&todoStore{}).handler()))
}
