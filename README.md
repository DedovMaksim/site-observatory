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

## Diagnostics (v0.3 beta)

The detail page now includes:

- **SSL/TLS:** certificate verification (trusted chain and hostname), UTC expiry, days left and a warning when fewer than 30 days remain. HTTP-only monitors show N/A.
- **DNS:** public A/AAAA addresses and a change indicator relative to the preceding diagnostic snapshot.
- **SEO:** bounded HTTP GET (max 512 KiB of decoded body) to extract the first HTML title, rel=canonical and robots meta. Missing title or meta noindex is shown as a warning.
- **Statistics:** rolling 24-hour sample-based uptime percentage and mean latency, plus an SVG graph for the most recent 100 stored HTTP checks.

Diagnostics are run separately from uptime checks. They start on application launch,
revisit enabled monitors approximately hourly, and discover newly added monitors
on the next minute tick. Diagnostic failures do **not** change the monitor's
UP/DOWN status; they are displayed on its detail page. Certificate expiry,
DNS and SEO diagnostics currently appear **in the web UI only**; Telegram alert
delivery for these conditions is reserved for a later iteration. DNS monitoring
in this release covers resolved A/AAAA addresses, not MX/NS/TXT or domain expiry.

The SEO parser is intentionally lightweight, not a full browser or JavaScript
renderer. It will not detect client-side changes rendered by JavaScript.

This beta is intended for local testing before deployment to a public VDS.
An authenticated administrator can initiate checks of external sites. The
diagnostic HTTP and TLS clients block private/loopback addresses and re-resolve
redirect destinations. Review network egress restrictions in production.


## Domain expiry monitoring (v0.4 beta)

Domain monitoring is independent of uptime checks. Every monitored HTTP(S)
host is mapped to a registration domain; multiple subdomains share the same
registration record. Existing monitors are mapped during database startup
migration. New/edited monitors are mapped as they are saved.

- **Automatic:** IANA RDAP bootstrap chooses an HTTPS server; RDAP `registrar
  expiration` is preferred over `expiration` when both are available.
  Checks run approximately daily, with bounded response size and timeouts.
- **Manual override:** Set the expiry date when adding/editing a monitor or at
  `/domains`; the entered date has priority over automatic RDAP data.
  Clear the date at `/domains` to return to RDAP.
- **Alerts:** With Telegram notifications enabled and configured, domain
  expiration thresholds are 30, 14, 7 and 1 day plus an overdue warning.
  Each threshold is queued once for a given domain/expiry-date pair.
  The queue uses the existing durable Telegram outbox.
- **Unknown:** If no supported RDAP server or expiration event is available,
  the app shows the error and does not invent an expiration date. Previous
  successful RDAP data is retained on transient query errors.

**Important limitations:** The .ru registry is not listed in the public IANA RDAP bootstrap. For .ru
and .рф (xn--p1ai), the program falls back to the official TCI WHOIS
server at whois.tcinet.ru:43 and reads the paid-till field (not free-date).
If outbound TCP port 43 is blocked or the registry rate-limits queries,
set a manual date from the registrar. RDAP registry expiry can
differ from a registrar's paid-through date; manual override exists for this
reason. Registrable domain extraction recognizes common multi-part public
suffixes, but is not a full Public Suffix List implementation, so inspect
complex country-code/private suffixes before relying on notifications.

This implementation is **beta**: back up `data/` before upgrade, review
results at `/domains`, test the alert schedule and do not rely on it as
the only source of renewal reminders.

### WHOIS fallback for .ru / .рф

When an RDAP lookup fails for a .ru or .рф domain, Site Observatory queries the fixed official TCI WHOIS endpoint over TCP port 43 and extracts `paid-till`. The same hourly scheduler checks for previously unsupported RDAP entries so older .ru monitors get another lookup shortly after upgrading. Subsequent successful lookups run approximately once a day. Responses are capped at 64 KiB and TCP requests have timeouts. WHOIS does not replace manual expiry dates, which take priority. If your network blocks port 43, manual dates remain available.


## Sitemap monitoring (v0.5 beta)

Navigate to `/sitemaps` and opt in per monitor. Disabled by default to
avoid extra traffic to websites. Enter the sitemap URL (e.g. `/sitemap.xml`,
`/sitemap_index.xml`, or `/wp-sitemap.xml`). Blank uses
`<monitored-origin>/sitemap.xml`.

A successful daily scan reads sitemap XML and sitemap indexes, recursively
follows same-host nested sitemap links, records the number of XML files and
unique page URLs, and compares the current URL list with the preceding
successful scan. First scan establishes a **baseline** (zero changes).
Later scans report added URLs and URLs removed from the sitemap. A removed
URL does **not** necessarily indicate a deleted webpage.

A failure leaves the prior successful URL snapshot intact and shows the
error in the interface. Results are available on `/sitemaps`, the site
detail page, and `/sitemaps/<id>/changes`.

**Limits in v0.5:** 60 sitemap XML files, 20,000 unique URLs, 12 MiB
uncompressed content per XML file, HTTPS/HTTP only, no cross-host sitemap
links or page URLs. Larger or unusual sitemaps receive an explicit error;
no partial snapshot is written. Files may be .gz compressed. Sitemap
checking is once daily, with newly enabled monitors picked up on the
next minute. This release **does not crawl each page**, perform per-page
SEO audits, or send sitemap alerts to Telegram.

Security: sitemap URLs must stay on the monitored hostname, outbound
connections to private/loopback IPs are blocked, DNS is re-validated before
each connection, and redirects are restricted to the same hostname.
The app stores URL strings and a bounded 500-event change history per
monitor rather than full page HTML.

Back up `data/` before first startup with this version. Local testing is
recommended before deploying to a VDS.
