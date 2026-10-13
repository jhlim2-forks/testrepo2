package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Todo struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Due   string `json:"due,omitempty"`
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
	title := ""
	if raw, exists := input["title"]; exists {
		var decodedTitle any
		if err := json.Unmarshal(raw, &decodedTitle); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "요청 형식이 올바르지 않습니다."})
			return
		}
		var ok bool
		if title, ok = decodedTitle.(string); !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "요청 형식이 올바르지 않습니다."})
			return
		}
	}
	due := ""
	if raw, exists := input["due"]; exists {
		var decodedDue any
		if err := json.Unmarshal(raw, &decodedDue); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "요청 형식이 올바르지 않습니다."})
			return
		}
		if decodedDue != nil {
			value, ok := decodedDue.(string)
			if !ok {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "요청 형식이 올바르지 않습니다."})
				return
			}
			due = value
		}
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
	if due != "" && !validDueDate(due) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "마감일은 실제 있는 날짜를 YYYY-MM-DD 형식으로 입력해 주세요."})
		return
	}
	s.mu.Lock()
	item := Todo{ID: len(s.items) + 1, Title: title, Due: due}
	s.items = append(s.items, item)
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, item)
}

// validDueDate reports whether due is a YYYY-MM-DD string naming a real calendar date.
func validDueDate(due string) bool {
	if len(due) != 10 {
		return false
	}
	for i := 0; i < len(due); i++ {
		if i == 4 || i == 7 {
			if due[i] != '-' {
				return false
			}
		} else if due[i] < '0' || due[i] > '9' {
			return false
		}
	}
	_, err := time.Parse("2006-01-02", due)
	return err == nil
}

func main() {
	listen := flag.String("listen", "127.0.0.1:8091", "HTTP listen address")
	flag.Parse()
	log.Fatal(http.ListenAndServe(*listen, (&todoStore{}).handler()))
}
