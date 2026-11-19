package web

import (
    "net/http"
    "net/http/httptest"
    "path/filepath"
    "strings"
    "testing"

    "example.com/site-observatory/internal/auth"
    "example.com/site-observatory/internal/monitor"
    "example.com/site-observatory/internal/notify"
    "example.com/site-observatory/internal/storage"
)

func TestLoginHidesAuthenticatedNavigation(t *testing.T) {
    dataDir := t.TempDir()
    db, err := storage.Open(filepath.Join(dataDir, "test.db"))
    if err != nil { t.Fatal(err) }
    defer db.Close()

    manager, err := auth.New(dataDir, false)
    if err != nil { t.Fatal(err) }
    tg, err := notify.New(db, dataDir)
    if err != nil { t.Fatal(err) }
    app, err := New(db, manager, monitor.Checker{}, tg)
    if err != nil { t.Fatal(err) }

    req := httptest.NewRequest(http.MethodGet, "/login", nil)
    rec := httptest.NewRecorder()
    app.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("login page returned status %d", rec.Code)
    }
    html := rec.Body.String()
    if !strings.Contains(html, "<h1>Вход</h1>") {
        t.Fatal("login form is missing")
    }
    for _, unwanted := range []string{
        `href="/incidents"`,
        `href="/settings"`,
        `href="/monitors/new"`,
        `action="/logout"`,
    } {
        if strings.Contains(html, unwanted) {
            t.Errorf("login page exposes authenticated control %q", unwanted)
        }
    }
}
