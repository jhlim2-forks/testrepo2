package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Todo struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Due   string `json:"due,omitempty"`
	Done  bool   `json:"done,omitempty"`
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
	mux.HandleFunc("/api/todos/", s.todo)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// respondError writes the {"error": "..."} body shared by every failed API response.
func respondError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (s *todoStore) todos(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.mu.Lock()
		items := append([]Todo{}, s.items...)
		s.mu.Unlock()
		sort.SliceStable(items, func(i, j int) bool { return !items[i].Done && items[j].Done })
		writeJSON(w, http.StatusOK, map[string]any{"todos": items})
		return
	}
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "지원하지 않는 요청 방식입니다.")
		return
	}
	var input map[string]json.RawMessage
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		respondError(w, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.")
		return
	}
	title := ""
	if raw, exists := input["title"]; exists {
		var decodedTitle any
		if err := json.Unmarshal(raw, &decodedTitle); err != nil {
			respondError(w, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.")
			return
		}
		var ok bool
		if title, ok = decodedTitle.(string); !ok {
			respondError(w, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.")
			return
		}
	}
	due := ""
	if raw, exists := input["due"]; exists {
		var decodedDue any
		if err := json.Unmarshal(raw, &decodedDue); err != nil {
			respondError(w, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.")
			return
		}
		if decodedDue != nil {
			value, ok := decodedDue.(string)
			if !ok {
				respondError(w, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.")
				return
			}
			due = value
		}
	}
	title, message := cleanTitle(title)
	if message != "" {
		respondError(w, http.StatusBadRequest, message)
		return
	}
	if due != "" && !validDueDate(due) {
		respondError(w, http.StatusBadRequest, "마감일은 실제 있는 날짜를 YYYY-MM-DD 형식으로 입력해 주세요.")
		return
	}
	s.mu.Lock()
	item := Todo{ID: len(s.items) + 1, Title: title, Due: due}
	s.items = append(s.items, item)
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, item)
}

// todo handles PATCH /api/todos/{id}, which marks a todo as done or not done and edits its title.
func (s *todoStore) todo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		respondError(w, http.StatusMethodNotAllowed, "지원하지 않는 요청 방식입니다.")
		return
	}
	id, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/todos/"))
	s.mu.Lock()
	found := err == nil && id >= 1 && id <= len(s.items)
	s.mu.Unlock()
	if !found {
		respondError(w, http.StatusNotFound, "할 일을 찾을 수 없습니다.")
		return
	}
	var input map[string]json.RawMessage
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&input); err != nil {
		respondError(w, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		respondError(w, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.")
		return
	}
	rawDone, hasDone := input["done"]
	rawTitle, hasTitle := input["title"]
	if !hasDone && !hasTitle {
		respondError(w, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.")
		return
	}
	var done bool
	if hasDone {
		var decodedDone any
		var ok bool
		if json.Unmarshal(rawDone, &decodedDone) != nil {
			respondError(w, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.")
			return
		}
		if done, ok = decodedDone.(bool); !ok {
			respondError(w, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.")
			return
		}
	}
	var title string
	if hasTitle {
		var decodedTitle any
		var ok bool
		if json.Unmarshal(rawTitle, &decodedTitle) != nil {
			respondError(w, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.")
			return
		}
		if title, ok = decodedTitle.(string); !ok {
			respondError(w, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.")
			return
		}
		var message string
		if title, message = cleanTitle(title); message != "" {
			respondError(w, http.StatusBadRequest, message)
			return
		}
	}
	s.mu.Lock()
	if hasDone {
		s.items[id-1].Done = done
	}
	if hasTitle {
		s.items[id-1].Title = title
	}
	item := s.items[id-1]
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, item)
}

// cleanTitle trims a title and checks the rules shared by adding and editing a todo.
// It returns the trimmed title, or the Korean error message when the title is rejected.
func cleanTitle(title string) (string, string) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", "제목을 입력해 주세요."
	}
	if len([]rune(title)) > 200 {
		// REQ-04, REQ-17: 글자 수는 앞뒤 공백을 제거한 제목의 유니코드 코드 포인트 수다.
		return "", "제목은 200자까지 입력할 수 있습니다."
	}
	return title, ""
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
