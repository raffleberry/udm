package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/raffleberry/udm/internal/cfg"
	"github.com/raffleberry/udm/internal/core"
)

// stub satisfies the three narrow core interfaces the routes consume, so the
// HTTP layer can be tested without a service, a downloader or a disk.
type stub struct {
	jobs  []core.Job
	added []core.Req
	acts  []string
}

func (s *stub) Add(r core.Req) (core.Job, error) {
	s.added = append(s.added, r)
	j := core.Job{ID: "new", URL: r.URL, Name: r.Name, State: core.Queued}
	s.jobs = append(s.jobs, j)
	return j, nil
}

func (s *stub) List() []core.Job { return s.jobs }

func (s *stub) record(op string, ids ...string) error {
	s.acts = append(s.acts, op+":"+strings.Join(ids, ","))
	return nil
}

func (s *stub) Start(ids ...string) error  { return s.record("start", ids...) }
func (s *stub) Pause(ids ...string) error  { return s.record("pause", ids...) }
func (s *stub) Stop(ids ...string) error   { return s.record("stop", ids...) }
func (s *stub) Remove(ids ...string) error { return s.record("remove", ids...) }

func serve(t *testing.T, s *stub) *Server {
	t.Helper()
	srv, err := New(0, s, s, s, &cfg.Config{Port: 0, Dir: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Stop)
	return srv
}

func do(t *testing.T, srv *Server, method, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, r)
	return w
}

func TestPing(t *testing.T) {
	srv := serve(t, &stub{})
	w := do(t, srv, http.MethodGet, "/api/ping", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", w.Code)
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["ok"] != true || got["name"] != Name {
		t.Errorf("ping = %v", got)
	}
}

func TestAdd(t *testing.T) {
	s := &stub{}
	srv := serve(t, s)

	w := do(t, srv, http.MethodPost, "/api/add",
		`{"url":"https://e.com/a.zip","name":"a.zip","referrer":"https://e.com"}`, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201: %s", w.Code, w.Body)
	}
	if len(s.added) != 1 || s.added[0].URL != "https://e.com/a.zip" {
		t.Errorf("Add got %+v", s.added)
	}
}

// An empty ids list is the Stop All button: it must reach every job.
func TestEmptyIDsMeansEveryJob(t *testing.T) {
	s := &stub{jobs: []core.Job{{ID: "a"}, {ID: "b"}, {ID: "c"}}}
	srv := serve(t, s)

	if w := do(t, srv, http.MethodPost, "/api/stop", `{"ids":[]}`, nil); w.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200: %s", w.Code, w.Body)
	}
	if len(s.acts) != 1 || s.acts[0] != "stop:a,b,c" {
		t.Errorf("acts = %v, want [stop:a,b,c]", s.acts)
	}
}

func TestIDsArePassedThrough(t *testing.T) {
	s := &stub{jobs: []core.Job{{ID: "a"}, {ID: "b"}}}
	srv := serve(t, s)

	do(t, srv, http.MethodPost, "/api/pause", `{"ids":["b"]}`, nil)
	if len(s.acts) != 1 || s.acts[0] != "pause:b" {
		t.Errorf("acts = %v, want [pause:b]", s.acts)
	}
}

func TestJobsListsThem(t *testing.T) {
	s := &stub{jobs: []core.Job{{ID: "a", Name: "a.zip", State: core.Running}}}
	srv := serve(t, s)

	w := do(t, srv, http.MethodGet, "/api/jobs", "", nil)
	var got []core.Job
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "a.zip" || got[0].State != core.Running {
		t.Errorf("jobs = %+v", got)
	}
}

// A web page must not be able to drive the download manager just by knowing the
// port. An extension sends a moz-extension origin; anything else is refused.
func TestForeignOriginIsRefused(t *testing.T) {
	s := &stub{}
	srv := serve(t, s)

	w := do(t, srv, http.MethodPost, "/api/add", `{"url":"https://e.com/x"}`,
		map[string]string{"Origin": "https://evil.example"})
	if w.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403", w.Code)
	}
	if len(s.added) != 0 {
		t.Error("a refused request still reached the core")
	}
}

func TestExtensionOriginIsAllowed(t *testing.T) {
	s := &stub{}
	srv := serve(t, s)
	origin := "moz-extension://4a1b-uuid"

	w := do(t, srv, http.MethodPost, "/api/add", `{"url":"https://e.com/x"}`,
		map[string]string{"Origin": origin})
	if w.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201: %s", w.Code, w.Body)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Errorf("Allow-Origin = %q, want %q", got, origin)
	}
}

// The native host sends no Origin at all, and must be let through.
func TestNoOriginIsAllowed(t *testing.T) {
	s := &stub{}
	srv := serve(t, s)

	w := do(t, srv, http.MethodPost, "/api/add", `{"url":"https://e.com/x"}`, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201: %s", w.Code, w.Body)
	}
}

func TestPreflightIsAnswered(t *testing.T) {
	srv := serve(t, &stub{})
	w := do(t, srv, http.MethodOptions, "/api/add", "",
		map[string]string{"Origin": "moz-extension://x"})
	if w.Code != http.StatusNoContent {
		t.Errorf("code = %d, want 204", w.Code)
	}
}
