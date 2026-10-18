package main

import (
	"context"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"trustissue/trustctl/internal/protocol"
)

//go:embed web/index.html
var indexHTML []byte

//go:embed web/jsQR.js
var jsQRSource []byte

//go:embed web/app.js
var appSource []byte

//go:embed web/jsQR-LICENSE.txt
var jsQRLicense []byte

// server is the local web page's back end. The page (in the browser) sees the camera and reads the
// QR codes; the export text goes here, is decoded here, and the secrets never go back to the page:
// it only gets names and reasons. Keys are stored by running `trustctl add` with the secret on stdin.
type server struct {
	token    string   // the page sends it in X-Token; it is in the URL that is opened
	host     string   // the only Host header that is served (the loopback address and port)
	trustctl []string // the command in front of "add": the binary and the options for it

	mu       sync.Mutex
	col      *collector
	accounts []account
	complete bool
	rows     map[int]rowState // import progress by account number
	done     map[int]bool     // stored already, so a retry cannot store them twice
	running  bool
	lastUse  time.Time
	quit     chan struct{}
	quitOnce sync.Once
}

type rowState struct {
	State string `json:"state"` // "working", "ok", "denied" or "error"
	Text  string `json:"text"`
}

func newServer(token, host string, trustctl []string) *server {
	return &server{token: token, host: host, trustctl: trustctl, col: newCollector(),
		rows: map[int]rowState{}, done: map[int]bool{}, lastUse: time.Now(), quit: make(chan struct{})}
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.guard(false, s.index))
	mux.HandleFunc("GET /jsQR.js", s.guard(false, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Write(jsQRSource)
	}))
	mux.HandleFunc("GET /jsQR-LICENSE.txt", s.guard(false, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write(jsQRLicense)
	}))
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /app.js", s.guard(false, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Write(appSource)
	}))
	mux.HandleFunc("GET /api/state", s.guard(true, s.state))
	mux.HandleFunc("POST /api/scan", s.guard(true, s.scan))
	mux.HandleFunc("POST /api/reset", s.guard(true, s.reset))
	mux.HandleFunc("POST /api/import", s.guard(true, s.importKeys))
	mux.HandleFunc("POST /api/quit", s.guard(true, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		s.stop()
	}))
	return mux
}

func (s *server) stop() { s.quitOnce.Do(func() { close(s.quit) }) }

// guard keeps everything but the browser tab we opened out: a DNS-rebinding page has the wrong
// Host, a page of another site has the wrong Origin, and anything without the token (the API)
// is refused.
func (s *server) guard(needToken bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Host != s.host {
			http.Error(w, "wrong host", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+s.host {
			http.Error(w, "wrong origin", http.StatusForbidden)
			return
		}
		if needToken && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Token")), []byte(s.token)) != 1 {
			http.Error(w, "missing or wrong token", http.StatusForbidden)
			return
		}
		s.mu.Lock()
		s.lastUse = time.Now()
		s.mu.Unlock()
		w.Header().Set("Cache-Control", "no-store")
		next(w, r)
	}
}

func (s *server) index(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("t")), []byte(s.token)) != 1 {
		http.Error(w, "open the address that the program printed (it has the token)", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; media-src 'self' blob:; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'")
	w.Write(indexHTML)
}

type accountView struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Supported bool   `json:"supported"`
	Reason    string `json:"reason,omitempty"`
}

type stateView struct {
	Phase    string           `json:"phase"` // "scan" or "review"
	Have     int              `json:"have"`
	Size     int              `json:"size"`
	Accounts []accountView    `json:"accounts,omitempty"`
	Running  bool             `json:"running"`
	Rows     map[int]rowState `json:"rows"`
	Done     map[int]bool     `json:"done"`
}

// view describes the state for the page: names and reasons, never secrets. Callers hold s.mu.
func (s *server) view() stateView {
	v := stateView{Phase: "scan", Running: s.running, Rows: map[int]rowState{}, Done: map[int]bool{}}
	v.Have, v.Size = s.col.progress()
	if len(s.col.parts) == 0 {
		v.Have = 0
	}
	for id, st := range s.rows {
		v.Rows[id] = st
	}
	for id := range s.done {
		v.Done[id] = true
	}
	if s.complete {
		v.Phase = "review"
		for i, a := range s.accounts {
			reason := a.unsupported()
			v.Accounts = append(v.Accounts, accountView{ID: i, Name: a.defaultName(), Supported: reason == "", Reason: reason})
		}
	}
	return v
}

func (s *server) state(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, s.view())
}

func (s *server) scan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	b, err := parseMigration(req.Text)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if s.complete { // a finished scan is only replaced after "scan again"
		writeJSON(w, http.StatusOK, s.view())
		return
	}
	if s.col.add(b) {
		s.accounts = s.col.accounts()
		s.complete = true
	}
	writeJSON(w, http.StatusOK, s.view())
}

func (s *server) reset(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "an import is running"})
		return
	}
	s.col, s.accounts, s.complete = newCollector(), nil, false
	s.rows, s.done = map[int]rowState{}, map[int]bool{}
	writeJSON(w, http.StatusOK, s.view())
}

type importItem struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func (s *server) importKeys(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Items []importItem `json:"items"`
	}
	if err := readJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "an import is running"})
		return
	}
	problems := map[int]string{}
	seen := map[string]int{}
	var jobs []job
	for _, it := range req.Items {
		switch {
		case !s.complete || it.ID < 0 || it.ID >= len(s.accounts):
			problems[it.ID] = "no such account"
			continue
		case s.done[it.ID]:
			continue // stored already
		case s.accounts[it.ID].unsupported() != "":
			problems[it.ID] = s.accounts[it.ID].unsupported()
			continue
		}
		if err := protocol.ValidateName(it.Name); err != nil {
			problems[it.ID] = err.Error()
			continue
		}
		lower := strings.ToLower(it.Name)
		if _, dup := seen[lower]; dup {
			problems[it.ID] = "the same name is used twice"
			continue
		}
		seen[lower] = it.ID
		jobs = append(jobs, job{id: it.ID, name: it.Name, uri: s.accounts[it.ID].uri()})
	}
	if len(problems) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "some names are not acceptable", "problems": problems})
		return
	}
	if len(jobs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nothing selected"})
		return
	}
	s.running = true
	for _, j := range jobs {
		delete(s.rows, j.id)
	}
	go func() {
		storeKeys(s.trustctl, jobs, func(id int, state, text string) {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.rows[id] = rowState{State: state, Text: text}
			if state == "ok" {
				s.done[id] = true
			}
		})
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()
	writeJSON(w, http.StatusAccepted, map[string]bool{"started": true})
}

type job struct {
	id        int
	name, uri string
}

// storeKeys runs `trustctl add NAME` for each job with the URI on stdin. report gets "working",
// "ok", "denied" or "error". It stops after a denial or timeout on the device and when there is
// no device (asking for the next key would only repeat it); any other error goes on to the next key.
// It returns true when every key was stored.
func storeKeys(command []string, jobs []job, report func(id int, state, text string)) bool {
	all := true
	for _, j := range jobs {
		report(j.id, "working", "hold the button on the device for 1 to 2 seconds")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		cmd := exec.CommandContext(ctx, command[0], append(append([]string(nil), command[1:]...), "add", j.name)...)
		cmd.Stdin = strings.NewReader(j.uri + "\n")
		var stderr strings.Builder
		cmd.Stderr = &stderr
		err := cmd.Run()
		cancel()
		if err == nil {
			report(j.id, "ok", "stored")
			continue
		}
		all = false
		message := lastLine(stderr.String())
		var exit *exec.ExitError
		if !errors.As(err, &exit) { // could not run it at all, or killed by the timeout
			report(j.id, "error", err.Error())
			return false
		}
		switch exit.ExitCode() {
		case 2:
			report(j.id, "denied", orDefault(message, "denied on the device"))
			return false
		case 69:
			report(j.id, "error", orDefault(message, "no device"))
			return false
		}
		report(j.id, "error", orDefault(message, fmt.Sprintf("trustctl failed (exit %d)", exit.ExitCode())))
	}
	return all
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return strings.TrimPrefix(strings.TrimSpace(lines[len(lines)-1]), "trustctl: ")
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func readJSON(r *http.Request, v any) error {
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v); err != nil {
		return errors.New("bad request")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
