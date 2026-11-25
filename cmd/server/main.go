package main

import (
	"context"
	"database/sql"
	"errors"
	"example.com/site-observatory/internal/auth"
	"example.com/site-observatory/internal/config"
	"example.com/site-observatory/internal/diagnostics"
	"example.com/site-observatory/internal/domainwatch"
	"example.com/site-observatory/internal/monitor"
	"example.com/site-observatory/internal/notify"
	"example.com/site-observatory/internal/scheduler"
	"example.com/site-observatory/internal/sitemapwatch"
	"example.com/site-observatory/internal/robotswatch"
	"example.com/site-observatory/internal/storage"
	"example.com/site-observatory/internal/web"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	cfg := config.Load()
	if e := cfg.Validate(); e != nil {
		log.Fatal(e)
	}
	db, e := storage.Open(filepath.Join(cfg.DataDir, "observatory.db"))
	if e != nil {
		log.Fatal(e)
	}
	defer db.Close()
	_, e = db.UserHash()
	if errors.Is(e, sql.ErrNoRows) {
		if cfg.AdminPassword == "" {
			log.Fatal("First start: set ADMIN_PASSWORD (at least 12 characters) in .env")
		}
		h, err := auth.HashPassword(cfg.AdminPassword)
		if err != nil {
			log.Fatal(err)
		}
		if err = db.CreateAdmin(h); err != nil {
			log.Fatal(err)
		}
		log.Print("Admin user initialized; remove ADMIN_PASSWORD from environment")
	} else if e != nil {
		log.Fatal(e)
	}
	mgr, e := auth.New(cfg.DataDir, cfg.CookieSecure)
	if e != nil {
		log.Fatal(e)
	}
	checker := monitor.Checker{AllowPrivate: cfg.AllowPrivateTargets}
	telegram, e := notify.New(db, cfg.DataDir)
	if e != nil { log.Fatal(e) }
	app, e := web.New(db, mgr, checker, telegram)
	if e != nil {
		log.Fatal(e)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go scheduler.New(db, checker).Run(ctx)
	go diagnostics.NewRunner(db).Run(ctx)
	go domainwatch.NewRunner(db).Run(ctx)
	go sitemapwatch.New(db).Run(ctx)
	go robotswatch.New(db).Run(ctx)
	go telegram.Run(ctx)
	srv := &http.Server{Addr: cfg.Addr, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 25 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		stop, finish := context.WithTimeout(context.Background(), 10*time.Second)
		defer finish()
		_ = srv.Shutdown(stop)
	}()
	log.Printf("Site Observatory listening at %s", cfg.Addr)
	if e = srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		log.Fatal(e)
	}
}
