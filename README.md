# Site Observatory

Self-hosted site uptime monitoring. **Go 1.27 + SQLite + server-rendered HTML + lightweight JavaScript**. Single binary, dark/light UI, login, HTTP checks, incident tracking and a Docker/systemd deployment path.

> Development snapshot v0.1: source provided for review. External dependency downloads were blocked in the generation environment; full integration build and cross-platform smoke tests still need to be completed. This is **not** a production-certified release.

## Features

- Monitor HTTP/HTTPS URLs from a Go worker pool (up to 10 concurrent checks).
- Configurable check interval (15–86400 s) and timeout (1–60 s).
- HTTP 200–399 considered healthy; 3 consecutive failures => DOWN; 2 consecutive successes => UP.
- SQLite in WAL mode. 30-day detailed check retention; incident history is not automatically deleted.
- Server-rendered dashboard, per-monitor history, incidents, settings/edit form, private `/api/status` JSON.
- Single admin account initialized from `ADMIN_PASSWORD` on first startup, PBKDF2 password verification, signed 24-hour sessions, CSRF tokens.
- URL validation and per-connection DNS resolution with public-address enforcement (private/loopback targets blocked by default). Redirects validated too.
- Multi-platform build workflow, Docker Compose, example Nginx + systemd, SQLite online-backup script.

## Quick start on Windows

Install Go 1.27+ and Git. Open PowerShell:

```powershell
Copy-Item .env.example .env
notepad .env
# Replace ADMIN_PASSWORD with a long random password.
go mod tidy
go test ./...
go run ./cmd/server
```

Visit http://127.0.0.1:8080, username `admin`, password from `.env`. The app creates `data/observatory.db` and `data/session.key`. After first login, remove `ADMIN_PASSWORD` from `.env` (the database keeps the password hash). Don't commit `.env` or `data/`.

## Linux with systemd

1. `go mod tidy && CGO_ENABLED=0 go build -o site-observatory ./cmd/server`.
2. Create an unprivileged `observatory` OS account, `/opt/site-observatory` for binary, and writable `/var/lib/site-observatory` owned by that account.
3. Put `APP_ADDR=127.0.0.1:8080`, `DATA_DIR=/var/lib/site-observatory`, `ADMIN_PASSWORD=<random-long-password>`, `APP_PUBLIC_URL=https://monitor.example.org`, `COOKIE_SECURE=true` into `/etc/site-observatory.env` (chmod 600). Remove initial password from file after first run.
4. Install `deploy/site-observatory.service` into `/etc/systemd/system/`, `systemctl daemon-reload`, `systemctl enable --now site-observatory`.
5. Configure Nginx using `deploy/nginx.conf.example`, replace domain and obtain a TLS certificate with a CA client such as Certbot.
6. Run `scripts/backup.sh` as a scheduled task, with `DB_PATH` and `BACKUP_DIR` set to persistent paths. Keep backups off-host.

## Docker Compose

```bash
cp .env.example .env
# Change ADMIN_PASSWORD and set COOKIE_SECURE=true + APP_PUBLIC_URL=https://your-domain
# On Linux: go mod tidy (creates go.sum), then:
docker compose up -d --build
```

For Go dependencies run `go mod tidy` with network access first. Commit the resulting `go.sum` before CI/release.

The compose configuration exposes port 8080 **only on 127.0.0.1**; run a local reverse proxy with HTTPS. Named volume `observatory_data` persists database and session key. Docker's `.env` is still required by `env_file`, with `APP_ADDR` overridden to listen inside the container.

## Host compatibility

- Windows: local binary, browser UI.
- Linux VPS/VDS: recommended production target; systemd or Docker, fronted by HTTPS.
- Traditional shared PHP hosting: **not guaranteed**. Requires continuous Go processes / socket binding; cron-only plans cannot provide the same always-on service.

## URLs and security

Do not expose the Go port directly to the Internet. Terminate TLS at a trusted reverse proxy; set `COOKIE_SECURE=true` and use a private admin password. Private targets are blocked by default and `ALLOW_PRIVATE_TARGETS=true` must be enabled only in a fully trusted single-user deployment. This is an MVP, **not** yet a multi-tenant SSRF-safe service. Reverse proxy authentication, rate limiting and operational monitoring are recommended for Internet-facing installations.

The session design is stateless: logging out deletes the browser cookie but does not revoke copies of a stolen cookie. Rotate `data/session.key` to invalidate all sessions (requires restart). There is currently no password-change screen, account lockout, email alerts, htmx fragment navigation, public status page, or multi-user system. Those are explicitly outside this 0.1 snapshot.

## Project map

- `cmd/server`: application entry point, graceful shutdown and admin bootstrap
- `internal/config`: `.env` and defaults
- `internal/storage`: SQLite schema and atomic check/incident state transitions
- `internal/monitor`: HTTP(S) probe, DNS/IP protections, response times
- `internal/scheduler`: timing and concurrency
- `internal/auth`: password hashes, sessions and CSRF
- `internal/web`: handlers, templates, CSS and JavaScript
- `deploy/`: VDS setup, reverse proxy and Dockerfile
- `.github/workflows/ci.yml`: tests, vet and cross-compiled artifacts
- `scripts/backup.sh`: consistent SQLite backup

## Follow-up roadmap

v0.2: Telegram notifications, SSL certificate expiry and DNS checks. v0.3: public status pages and export/API. v0.4: remote agents. Before public 1.0 release: add API/access hardening, login rate limiting, audit logging, HTMX partial refresh (or keep progressive native JS), e2e tests, packaging and backup restore drills.

License: not yet selected; add LICENSE before publishing as open source.

## Telegram Alerts (v0.2 beta)

Open **Settings** in the admin dashboard. Enter the bot token from @BotFather
and a numeric Telegram Chat ID, choose which transitions should trigger messages,
then save and click **Send test message**.

The bot must have been started by the recipient (or added to the target group).
Notifications are enqueued only when a monitor changes to **DOWN** after three
consecutive failed checks or recovers to **UP** after two successful checks.
The message worker retries delivery without blocking HTTP monitoring.

Bot tokens are stored encrypted (AES-256-GCM) in SQLite, never rendered back in
the browser. The encryption key is generated on first startup as
`DATA_DIR/telegram.key` and must be kept alongside the SQLite backup.
**Losing that key means existing bot tokens cannot be decrypted.** Both
`observatory.db` and `telegram.key` are secrets; never commit them to Git.
Stop the service before copying its data directory for backup, or use
SQLite's backup API for an online snapshot. Protect the web interface with HTTPS
and restrict admin access.

Changes to Telegram settings are made in the web interface; no Telegram bot
token is required in `.env`.
