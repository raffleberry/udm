// Package api exposes the core over HTTP on loopback. It is the only channel
// the browser extension and the native messaging host use.
//
// The server binds 127.0.0.1 and rejects any request carrying an Origin it
// does not recognise, so a random web page cannot drive it: a Firefox extension
// sends "moz-extension://<uuid>" and the native host sends no Origin at all.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/raffleberry/udm/internal/cfg"
	"github.com/raffleberry/udm/internal/core"
)

// Name and Version identify this app to the extension.
const (
	Name    = "udm"
	Version = "1.0"
)

// Server adapts the core to HTTP. It holds the narrow core interfaces rather
// than the concrete Service, so the routes can be exercised against fakes.
type Server struct {
	Add  core.Adder
	Ctl  core.Controller
	List core.Lister
	Cfg  *cfg.Config

	mux *http.ServeMux
	srv *http.Server
	ln  net.Listener
}

// New binds 127.0.0.1:port and returns a Server. Call Start, then Addr.
func New(port int, add core.Adder, ctl core.Controller, list core.Lister, c *cfg.Config) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return nil, err
	}
	s := &Server{Add: add, Ctl: ctl, List: list, Cfg: c, ln: ln}
	s.mux = s.routes()
	s.srv = &http.Server{
		Handler:           s.guard(s.mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s, nil
}

// Handler is the guarded router, exposed so the routes can be tested without
// binding a port.
func (s *Server) Handler() http.Handler { return s.srv.Handler }

// Addr is the bound address, which matters when port 0 was requested.
func (s *Server) Addr() string { return s.ln.Addr().String() }

// Start serves in the background.
func (s *Server) Start() {
	go func() {
		if err := s.srv.Serve(s.ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("api: serve", "err", err)
		}
	}()
	slog.Info("api: listening", "addr", s.Addr())
}

// Stop drains the server, bounded so a stuck client cannot block exit.
func (s *Server) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.srv.Shutdown(ctx); err != nil {
		slog.Warn("api: shutdown", "err", err)
	}
}

func (s *Server) routes() *http.ServeMux {
	m := &http.ServeMux{}
	m.HandleFunc("GET /api/ping", s.ping)
	m.HandleFunc("GET /api/jobs", s.jobs)
	m.HandleFunc("GET /api/settings", s.settings)
	m.HandleFunc("POST /api/add", s.add)

	// One handler shape for all four toolbar actions: {"ids": [".."]}.
	for path, act := range map[string]func(...string) error{
		"/api/start":  s.Ctl.Start,
		"/api/pause":  s.Ctl.Pause,
		"/api/stop":   s.Ctl.Stop,
		"/api/remove": s.Ctl.Remove,
	} {
		m.HandleFunc("POST "+path, s.act(act))
	}
	return m
}

func (s *Server) ping(w http.ResponseWriter, r *http.Request) {
	reply(w, http.StatusOK, map[string]any{"ok": true, "name": Name, "version": Version})
}

func (s *Server) jobs(w http.ResponseWriter, r *http.Request) {
	reply(w, http.StatusOK, s.List.List())
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	reply(w, http.StatusOK, s.Cfg)
}

type addReq struct {
	URL      string `json:"url"`
	Name     string `json:"name"`
	Referrer string `json:"referrer"`
}

func (s *Server) add(w http.ResponseWriter, r *http.Request) {
	var req addReq
	if !decode(w, r, &req) {
		return
	}
	job, err := s.Add.Add(core.Req{URL: req.URL, Name: req.Name, Referrer: req.Referrer})
	if err != nil {
		reply(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	reply(w, http.StatusCreated, job)
}

type actReq struct {
	IDs []string `json:"ids"`
}

// act adapts a Controller method to POST /api/<verb> {"ids": [...]}. An empty
// ids list means "every job", which is exactly what Stop All sends.
func (s *Server) act(fn func(...string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req actReq
		if !decode(w, r, &req) {
			return
		}
		ids := req.IDs
		if len(ids) == 0 {
			ids = idsOf(s.List.List())
		}
		if err := fn(ids...); err != nil {
			reply(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		reply(w, http.StatusOK, map[string]any{"ok": true, "n": len(ids)})
	}
}

// guard refuses foreign browsers and answers their preflights.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		org := r.Header.Get("Origin")
		if org == "" {
			next.ServeHTTP(w, r) // a local process: the native host, curl
			return
		}
		if !trusted(org) {
			slog.Warn("api: rejected foreign origin", "origin", org)
			reply(w, http.StatusForbidden, map[string]string{"error": "forbidden origin"})
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", org)
		w.Header().Set("Access-Control-Allow-Headers", "content-type")
		w.Header().Set("Access-Control-Allow-Methods", "get, post, options")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// trusted accepts extension pages only. The id is not pinned: a temporary
// add-on and a signed release have different uuids, and both are ours.
func trusted(org string) bool {
	const p = "moz-extension://"
	return len(org) > len(p) && org[:len(p)] == p
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(v); err != nil {
		reply(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return false
	}
	return true
}

func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("api: write", "err", err)
	}
}

func idsOf(js []core.Job) []string {
	ids := make([]string, len(js))
	for i, j := range js {
		ids[i] = j.ID
	}
	return ids
}
