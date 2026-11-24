package web

import (
	"database/sql"
	"math"
	"embed"
	"encoding/json"
	"errors"
	"example.com/site-observatory/internal/auth"
	"example.com/site-observatory/internal/monitor"
	"example.com/site-observatory/internal/notify"
	"example.com/site-observatory/internal/storage"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

//go:embed templates/*.html static/*
var assets embed.FS

type Server struct {
	DB        *storage.DB
	Auth      *auth.Manager
	Checker   monitor.Checker
	Notify    *notify.Telegram
	templates *template.Template
	mux       *http.ServeMux
}
type view struct {
	Title, CSRF, Error, Message string
	Monitors                    []storage.Monitor
	Monitor                     storage.Monitor
	Checks                      []storage.Check
	Incidents                   []storage.Incident
	ActiveIncidents             int
	Up, Down, Unknown           int
	Editing                     bool
	Telegram                    storage.TelegramSettings
	TokenConfigured             bool
	Diagnostic storage.Diagnostic
	HasDiagnostic bool
	Problems []storage.DiagnosticProblem
	Stats storage.UptimeStats
	GraphPoints string
}

func New(db *storage.DB, a *auth.Manager, c monitor.Checker, n *notify.Telegram) (*Server, error) {
	t, e := template.New("base").Funcs(template.FuncMap{"percent": func(x, y int) string {
		if y == 0 {
			return "0"
		}
		return fmt.Sprintf("%.1f", 100*float64(x)/float64(y))
	}, "short": func(s string) string {
		if len(s) > 130 {
			return s[:130] + "…"
		}
		return s
	}}).ParseFS(assets, "templates/*.html")
	if e != nil {
		return nil, e
	}
	s := &Server{DB: db, Auth: a, Checker: c, Notify: n, templates: t, mux: http.NewServeMux()}
	s.routes()
	return s, nil
}
func (s *Server) Handler() http.Handler { return securityHeaders(s.mux) }
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
func (s *Server) routes() {
	s.mux.Handle("GET /static/", http.FileServerFS(assets))
	s.mux.HandleFunc("GET /login", s.loginPage)
	s.mux.HandleFunc("POST /login", s.login)
	s.mux.HandleFunc("GET /", s.require(s.dashboard))
	s.mux.HandleFunc("GET /monitors/new", s.require(s.newForm))
	s.mux.HandleFunc("POST /monitors", s.require(s.create))
	s.mux.HandleFunc("GET /monitors/{id}", s.require(s.detail))
	s.mux.HandleFunc("GET /monitors/{id}/edit", s.require(s.editForm))
	s.mux.HandleFunc("POST /monitors/{id}/edit", s.require(s.update))
	s.mux.HandleFunc("POST /monitors/{id}/delete", s.require(s.delete))
	s.mux.HandleFunc("GET /incidents", s.require(s.incidents))
	s.mux.HandleFunc("GET /settings", s.require(s.settings))
	s.mux.HandleFunc("POST /settings/telegram", s.require(s.saveTelegram))
	s.mux.HandleFunc("POST /settings/telegram/test", s.require(s.testTelegram))
	s.mux.HandleFunc("GET /api/status", s.require(s.apiStatus))
	s.mux.HandleFunc("POST /logout", s.require(s.logout))
}
func (s *Server) token(r *http.Request) string {
	c, e := r.Cookie("so_session")
	if e != nil || !s.Auth.Valid(c.Value) {
		return ""
	}
	return c.Value
}
func (s *Server) require(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := s.token(r)
		if t == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if r.Method == http.MethodPost {
			if e := r.ParseForm(); e != nil || !s.Auth.ValidCSRF(t, r.PostForm.Get("csrf")) {
				http.Error(w, "invalid CSRF token", http.StatusForbidden)
				return
			}
			if r.Header.Get("Origin") != "" {
				u, e := url.Parse(r.Header.Get("Origin"))
				if e != nil || !strings.EqualFold(u.Host, r.Host) {
					http.Error(w, "invalid origin", http.StatusForbidden)
					return
				}
			}
		}
		h(w, r)
	}
}
func (s *Server) render(w http.ResponseWriter, r *http.Request, page string, v view) {
	if session := s.token(r); session != "" { v.CSRF = s.Auth.CSRF(session) } else { v.CSRF = "" }
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if e := s.templates.ExecuteTemplate(w, page, v); e != nil {
		log.Printf("render %s: %v", page, e)
	}
}
func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if s.token(r) != "" {
		http.Redirect(w, r, "/", 303)
		return
	}
	s.render(w, r, "login.html", view{Title: "Sign in"})
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if e := r.ParseForm(); e != nil {
		http.Error(w, "bad request", 400)
		return
	}
	hash, e := s.DB.UserHash()
	if e != nil {
		http.Error(w, "admin is not initialized", 503)
		return
	}
	if r.PostForm.Get("username") != "admin" || !auth.VerifyPassword(hash, r.PostForm.Get("password")) {
		time.Sleep(400 * time.Millisecond)
		w.WriteHeader(401)
		s.render(w, r, "login.html", view{Title: "Sign in", Error: "Неверный логин или пароль"})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "so_session", Value: s.Auth.Session(), HttpOnly: true, Secure: s.Auth.Secure, SameSite: http.SameSiteStrictMode, Path: "/", MaxAge: 86400})
	http.Redirect(w, r, "/", 303)
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "so_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.Auth.Secure, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/login", 303)
}
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	list, e := s.DB.Monitors()
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	v := view{Title: "Dashboard", Monitors: list}
	for _, m := range list {
		switch m.Status {
		case "UP":
			v.Up++
		case "DOWN":
			v.Down++
		default:
			v.Unknown++
		}
	}
	if problems,err:=s.DB.DiagnosticProblems();err==nil {v.Problems=problems}
	s.render(w, r, "dashboard.html", v)
}
func (s *Server) newForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "form.html", view{Title: "Новый монитор", Monitor: storage.Monitor{Interval: 60, Timeout: 10, Enabled: true}})
}
func parseMonitor(r *http.Request, c monitor.Checker) (storage.Monitor, error) {
	var m storage.Monitor
	if e := r.ParseForm(); e != nil {
		return m, e
	}
	m.Name = strings.TrimSpace(r.PostForm.Get("name"))
	m.URL = strings.TrimSpace(r.PostForm.Get("url"))
	m.Interval, _ = strconv.Atoi(r.PostForm.Get("interval"))
	m.Timeout, _ = strconv.Atoi(r.PostForm.Get("timeout"))
	m.Enabled = r.PostForm.Get("enabled") == "on"
	if len(m.Name) < 1 || len(m.Name) > 100 {
		return m, errors.New("название: от 1 до 100 символов")
	}
	if len(m.URL) > 2048 {
		return m, errors.New("URL слишком длинный")
	}
	if e := c.ValidateURL(m.URL); e != nil {
		return m, e
	}
	if m.Interval < 15 || m.Interval > 86400 {
		return m, errors.New("интервал: от 15 до 86400 секунд")
	}
	if m.Timeout < 1 || m.Timeout > 60 {
		return m, errors.New("тайм-аут: от 1 до 60 секунд")
	}
	return m, nil
}
func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	m, e := parseMonitor(r, s.Checker)
	if e != nil {
		w.WriteHeader(400)
		s.render(w, r, "form.html", view{Title: "Новый монитор", Monitor: m, Error: e.Error()})
		return
	}
	id, e := s.DB.CreateMonitor(m.Name, m.URL, m.Interval, m.Timeout, m.Enabled)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/monitors/%d", id), 303)
}
func id(r *http.Request) (int64, error) { return strconv.ParseInt(r.PathValue("id"), 10, 64) }
func (s *Server) fetch(w http.ResponseWriter, r *http.Request) (storage.Monitor, bool) {
	n, e := id(r)
	if e != nil {
		http.NotFound(w, r)
		return storage.Monitor{}, false
	}
	m, e := s.DB.Monitor(n)
	if e != nil {
		http.NotFound(w, r)
		return storage.Monitor{}, false
	}
	return m, true
}
func (s *Server) detail(w http.ResponseWriter, r *http.Request) {
	m, ok := s.fetch(w, r)
	if !ok {
		return
	}
	checks, e := s.DB.Checks(m.ID, 100)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	incidents, e := s.DB.Incidents(m.ID, 50)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	v := view{Title: m.Name, Monitor: m, Checks: checks, Incidents: incidents}
 for _,incident := range incidents { if incident.EndedAt == "" { v.ActiveIncidents++ } }
 if diag, err := s.DB.Diagnostic(m.ID); err == nil {v.Diagnostic=diag;v.HasDiagnostic=true}
 if stats, err := s.DB.Uptime(m.ID,time.Now().Add(-24*time.Hour)); err == nil {v.Stats=stats}
 if latencies,err:=s.DB.LastLatencies(m.ID,100);err==nil {v.GraphPoints=latencyGraph(latencies)}
 s.render(w,r,"detail.html",v)
}
func (s *Server) editForm(w http.ResponseWriter, r *http.Request) {
	m, ok := s.fetch(w, r)
	if !ok {
		return
	}
	s.render(w, r, "form.html", view{Title: "Редактировать", Monitor: m, Editing: true})
}
func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	old, ok := s.fetch(w, r)
	if !ok {
		return
	}
	m, e := parseMonitor(r, s.Checker)
	if e != nil {
		m.ID = old.ID
		w.WriteHeader(400)
		s.render(w, r, "form.html", view{Title: "Редактировать", Monitor: m, Editing: true, Error: e.Error()})
		return
	}
	if e = s.DB.UpdateMonitor(old.ID, m.Name, m.URL, m.Interval, m.Timeout, m.Enabled); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/monitors/%d", old.ID), 303)
}
func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	m, ok := s.fetch(w, r)
	if !ok {
		return
	}
	if e := s.DB.DeleteMonitor(m.ID); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	http.Redirect(w, r, "/", 303)
}
func (s *Server) incidents(w http.ResponseWriter, r *http.Request) {
	list, e := s.DB.Incidents(0, 200)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	s.render(w, r, "incidents.html", view{Title: "Инциденты", Incidents: list})
}
func (s *Server) apiStatus(w http.ResponseWriter, r *http.Request) {
	list, e := s.DB.Monitors()
	if e != nil {
		http.Error(w, "database error", 500)
		return
	}
	type item struct {
		ID      int64  `json:"id"`
		Name    string `json:"name"`
		Status  string `json:"status"`
		Code    int    `json:"code"`
		Latency int64  `json:"latency_ms"`
		Checked string `json:"checked_at"`
	}
	out := make([]item, 0, len(list))
	for _, m := range list {
		out = append(out, item{m.ID, m.Name, m.Status, m.LastCode, m.LastLatency, m.LastChecked})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

var _ = sql.ErrNoRows

func latencyGraph(series []int64)string {
 if len(series)==0{return ""}
 maxVal:=int64(1)
 for _,v:=range series {if v>maxVal {maxVal=v}}
 points:=make([]string,0,len(series))
 for i,v:=range series {
  x:=0
  if len(series)>1 {x=int(math.Round(float64(i)*500/float64(len(series)-1)))}
  y:=100-int(math.Round(float64(v)*95/float64(maxVal)))
  if y<0 {y=0}
  points=append(points,strconv.Itoa(x)+","+strconv.Itoa(y))
 }
 return strings.Join(points," ")
}
