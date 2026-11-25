# Site Observatory

**Self-hosted website monitoring, technical diagnostics and SEO change detection — built with Go and SQLite.**

Site Observatory is a lightweight web application for monitoring the availability and technical health of websites. It combines uptime checks, incident history, TLS/DNS/domain diagnostics, sitemap and robots.txt monitoring, and an optional page-level SEO audit. A read-only REST API and a local MCP server let external tools and AI assistants analyze the same monitoring data.

**Status:** v0.8 **Beta** · Go 1.27 · SQLite · server-rendered web UI · light/dark theme · single administrator

**Author:** [Maksim Dedov](https://maksimdedov.ru/) · [GitHub](https://github.com/DedovMaksim)

> **Beta software:** Tested locally on Windows and by automated Go CI, including Linux cross-builds. Production deployment, restore procedures and internet-facing hardening must be validated for your environment. Do not treat monitoring alerts as the only source of operational or domain-renewal reminders.

## Features

| Area | What it does |
| --- | --- |
| **Uptime & incidents** | Checks HTTP/HTTPS endpoints on a schedule, records response codes and latency, calculates 24-hour availability, tracks DOWN/UP incidents and shows charts |
| **Notifications** | Sends Telegram test messages, outage/recovery alerts and selected technical warnings via a durable queue |
| **TLS & DNS** | Tracks certificate verification and expiration, DNS A/AAAA addresses and detected changes |
| **Domain expiry** | Uses RDAP where available; falls back to TCI WHOIS `paid-till` for .ru/.рф; allows manual expiry overrides and renewal reminders |
| **Sitemap.xml** | Supports sitemap indexes, nested XML and gzip; detects added/removed URLs and maintains change history |
| **Robots.txt** | Watches availability and content changes; highlights potential root-disallow rules and sitemap-directive changes |
| **Page SEO audit** | Optionally checks URLs from sitemap for HTTP status, HTML title, canonical and robots directives, with searchable and filtered reports |
| **REST API** | Read-only JSON endpoints authenticated with separate Bearer API keys |
| **MCP** | Local stdio server exposing six read-only tools for AI agents, including GitHub Copilot in VS Code |

### How monitoring behaves

- **HTTP health:** HTTP 200–399 is considered healthy; three consecutive failures mark a monitor DOWN and two consecutive successes after an incident mark it UP.
- **Diagnostics:** TLS, DNS and basic SEO checks run independently of availability monitoring.
- **Domains:** Registration dates are checked approximately daily; renewal notifications use thresholds of 30, 14, 7 and 1 day when Telegram is configured.
- **Sitemap and robots.txt:** Opt-in monitoring, approximately daily, with bounded retrieval and change tracking.
- **SEO pages:** Opt-in only; up to **five pages per minute** across all monitored sites, at least **two seconds** between requests, and no page revisited sooner than **24 hours**. Metadata changes are logged. New HTTP errors, `noindex` and missing titles require a subsequent confirmation check before Telegram alerting. Ordinary title/canonical edits do not trigger Telegram.

This is static HTTP/HTML analysis, **not a browser-based crawler**. It does not execute JavaScript or guarantee how search engines will index a page. An intentional `noindex` or different canonical is not automatically a site fault.

## Quick start — Windows / Git Bash

**Requirements:** Go 1.27 or newer, Git, a modern browser.

```bash
git clone https://github.com/DedovMaksim/site-observatory.git
cd site-observatory
cp .env.example .env
```

Edit `.env`: set `ADMIN_PASSWORD` to a unique, long password (at least 12 characters). For local development, keep `APP_ADDR=127.0.0.1:8080` and `COOKIE_SECURE=false`.

```bash
go mod download
go test ./...
go run ./cmd/server
```

Open **http://127.0.0.1:8080** and sign in as `admin`. On first startup Site Observatory creates `data/observatory.db`, session-signing material and other local state. After the admin account is initialized, remove `ADMIN_PASSWORD` from `.env`; the password hash remains in SQLite.

**Keep** `.env`, `data/`, `telegram.key`, API keys and backups out of source control. An existing `data/` directory contains your monitoring history and should never be overwritten during a code update.

## REST API v1 (read-only)

The REST interface shares application services with the web UI and MCP, but uses **API Bearer keys**, not browser login cookies. There are no API write routes in v0.8.

To create a key, stop the running service and use the existing data directory:

```bash
go run ./cmd/apikey -data ./data -name local-client
```

The full credential is displayed **once**; store it securely. SQLite holds its SHA-256 hash, not the plaintext key. To revoke it, stop the service and run:

```bash
go run ./cmd/apikey -data ./data -name local-client -revoke
```

With the service running, test in Git Bash without adding a secret to shell history:

```bash
read -rsp "API key: " SO_API_KEY; echo
curl -fsS -H "Authorization: Bearer ${SO_API_KEY}" \
  http://127.0.0.1:8080/api/v1/summary
unset SO_API_KEY
```

| Method | Endpoint | Response |
| --- | --- | --- |
| GET | `/api/v1/summary` | Overall availability and open incident count |
| GET | `/api/v1/monitors` | Monitored sites and their most recent checks |
| GET | `/api/v1/monitors/{id}` | Detailed status: uptime, diagnostics, domain, sitemap, robots and page audit |
| GET | `/api/v1/incidents?monitor_id=1&limit=25` | Incident history (optional monitor filter) |
| GET | `/api/v1/domains` | Registration/renewal information |
| GET | `/api/v1/monitors/{id}/seo-issues?limit=25` | Current page-level HTTP/SEO findings |

Where supported, `limit` must be between 1 and 100. Never place a Bearer token in a URL. Network-facing use requires HTTPS and appropriate access controls and rate limiting.

## MCP server v0.8 (local stdio)

Site Observatory provides a separate **read-only MCP server** for locally trusted AI tools. It does not expose a remote MCP port.

Build a Windows executable from the repository root:

```bash
mkdir -p bin
go build -o bin/site-observatory-mcp.exe ./cmd/mcp
```

A VS Code workspace can use the following configuration in `.vscode/mcp.json` (**replace the paths with your own**):

```json
{
  "servers": {
    "site-observatory": {
      "type": "stdio",
      "command": "H:/Go/site-observatory/bin/site-observatory-mcp.exe",
      "args": ["-data", "H:/Go/site-observatory/data"]
    }
  }
}
```

In VS Code, start the server from the controls above the MCP configuration and enable its tools in your AI client. The example paths are local and must not be used for another machine without editing them. The MCP executable needs local read access to the existing SQLite data.

Available tools:

| MCP tool | Purpose |
| --- | --- |
| `get_summary` | Overall monitor health |
| `list_monitors` | Monitors and current status |
| `get_monitor_status` | Detailed diagnostics for a selected monitor |
| `list_incidents` | Recent incidents |
| `get_seo_issues` | Sitemap page SEO/HTTP findings |
| `get_domain_expiry` | Domain expiry information |

MCP uses the `2025-11-25` protocol lifecycle over stdio and supports `tools/list` and `tools/call`. AI tools have **no monitor, user or settings mutation capabilities** through this server. A local MCP client still has access to the underlying operating-system account's files; configure it only in trusted workspaces.

## Deploying to Linux VDS

The preferred production topology is **HTTPS → Nginx reverse proxy → Go service on 127.0.0.1:8080 → private SQLite directory**.

1. Create an unprivileged OS account and application directory; never run the application as root.
2. Build a Linux binary using Go or download a matching CI build artifact, and put it in `/opt/site-observatory`.
3. Create a private persistent directory such as `/var/lib/site-observatory`, owned by the service account. Configure `DATA_DIR` accordingly.
4. Use `deploy/site-observatory.service` as a starting point for systemd and provide a protected `/etc/site-observatory.env`.
5. Configure the HTTPS virtual host in ISPmanager/Nginx, forwarding traffic internally to the Go listener. The file `deploy/nginx.conf.example` is **an example**, not an instruction to overwrite an existing ISPmanager configuration.
6. Set `APP_PUBLIC_URL` to the actual HTTPS origin and `COOKIE_SECURE=true`. Restrict who can reach the admin panel and API.
7. Configure monitoring, Telegram and backups; test restoration before relying on the server.

**Do not expose port 8080 publicly.** On a VDS already hosting other websites, coordinate web-server changes with the existing administrator and avoid replacing global Nginx or Apache settings.

### Backup and recovery

SQLite uses WAL mode, so copying a live `.db` file on its own can produce an incomplete backup. Use the repository's `scripts/backup.sh` for a consistent SQLite snapshot, or stop the service and back up the entire data directory. Keep `telegram.key` and `session.key` with the database as applicable; protect off-host backups. Losing `telegram.key` prevents decryption of previously saved Telegram bot tokens.

```bash
DB_PATH=/var/lib/site-observatory/observatory.db \
BACKUP_DIR=/secure/backups \
bash scripts/backup.sh
```

The included `compose.yaml` offers an alternative container workflow. Review environment variables, persistent volume mounts and HTTPS proxy configuration before deployment.

## Configuration

| Variable | Meaning |
| --- | --- |
| `APP_ADDR` | HTTP listener, default `127.0.0.1:8080` |
| `DATA_DIR` | Persistent database and key files, default `./data` |
| `ADMIN_PASSWORD` | Initial admin bootstrap password; remove after first successful startup |
| `APP_PUBLIC_URL` | Public origin, use HTTPS behind the proxy |
| `COOKIE_SECURE` | Set to `true` when using HTTPS |
| `ALLOW_PRIVATE_TARGETS` | Default `false`; do not enable for untrusted or internet-facing installations |

Site Observatory enforces URL validation and public-IP restrictions for outgoing checks. As a single-admin **Beta**, it has not been independently security audited. Treat SSRF controls, account access, reverse-proxy rate limits and credential storage as production-critical.

## Repository layout

```text
cmd/
  server/              Web UI and background workers
  apikey/              Local API key provisioning
  mcp/                 Local MCP stdio executable
internal/
  api/                 REST v1 handlers
  auth/                Login, sessions and CSRF
  config/              Environment settings
  monitor/             HTTP checking
  observatory/         Shared read-only service for REST and MCP
  storage/             SQLite schema, state, alert histories
  web/                 Templates, CSS and web handlers
  ...                  Diagnostics, domain/sitemap/robots/SEO workers
deploy/                Example systemd, Nginx and Docker resources
scripts/               Maintenance and backup tools
.github/workflows/     CI, tests and cross-platform builds
```

## Development and project status

```bash
go vet ./...
go test -race ./...
go build ./cmd/server
go build ./cmd/mcp
```

GitHub Actions runs static checks, race-enabled tests and cross-builds for Windows AMD64, Linux AMD64 and Linux ARM64. **Passing CI does not replace production smoke tests.**

The project is currently an evolving **v0.8 Beta**. Future work may include hardened VDS deployment, backup/restore validation, richer audit reporting and additional monitoring integrations.

## Author and licensing

Developed by **[Maksim Dedov](https://maksimdedov.ru/)**. Source repository: [DedovMaksim/site-observatory](https://github.com/DedovMaksim/site-observatory).

**License:** not selected. The repository is currently private; no open-source licensing rights should be assumed. A LICENSE file and public release policy can be added separately if the project is published.


## Telegram HTTPS Relay (optional)

When a hosting provider cannot reach `api.telegram.org:443`, Site Observatory can send notifications through an existing, trusted HTTPS Relay. The default remains direct Telegram Bot API delivery.

Configure these two environment variables together (for a systemd deployment, put them in `/etc/site-observatory.env` with restricted permissions):

```dotenv
RELAY_URL=https://site-pinger-telegram.dedovmaxim.workers.dev
RELAY_SECRET=REPLACE_WITH_YOUR_PRIVATE_RELAY_SECRET
```

This Relay uses an authenticated `POST` request with `Authorization: Bearer <RELAY_SECRET>`, `Content-Type: application/json`, and the body `{ "text": "notification" }`. The Worker already stores its own Telegram Bot Token and Chat ID: Site Observatory does **not** send its local Bot Token or Chat ID to the Worker. The recipient is therefore configured **on the Worker side**. Relay mode is active only when both environment variables are set; a missing variable prevents service startup rather than silently falling back to direct delivery. Incoming error messages and logs do not include the secret, and HTTP redirects are not followed for Relay requests.

After updating the environment and restarting `site-observatory.service`, open the web **Settings** page and send a test message. Existing Telegram notification settings (enabled, incident and recovery flags), the durable outbox, retries and other site monitoring functions remain unchanged. **Never commit or share `RELAY_SECRET`**. Use a long unique secret and rotate it if exposed.
