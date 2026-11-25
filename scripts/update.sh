#!/usr/bin/env bash
# Run as root on a standard systemd deployment, using a trusted CI binary.
set -Eeuo pipefail
umask 077
[[ $EUID == 0 && $# == 3 ]] || { echo "Usage: sudo bash update.sh BINARY SHA256 LOCAL_HTTP_URL"; exit 2; }
candidate=$(realpath "$1")
digest=$2
probe=$3
bin=/opt/site-observatory/site-observatory
envfile=/etc/site-observatory.env
data=/var/lib/site-observatory
unit=site-observatory.service
[[ "$digest" =~ ^[a-fA-F0-9]{64}$ && "$probe" =~ ^http://127\.0\.0\.1:[0-9]+/login$ ]] || exit 2
[[ -f "$candidate" && "$candidate" != "$bin" && -f "$bin" && -f "$envfile" && -f "$data/observatory.db" ]]
printf '%s  %s\n' "$digest" "$candidate" | sha256sum -c -
exec 9>/run/lock/site-observatory-update.lock
flock -n 9
systemctl is-active --quiet "$unit"
curl --fail --silent --max-time 5 "$probe" >/dev/null
# Validate the fixed-layout deployment without sourcing executable shell text.
python3 - "$envfile" "$data" "$probe" <<'PY'
import sys
from pathlib import Path
p, data, probe = sys.argv[1:]
values = {}
for line in Path(p).read_text().splitlines():
    line = line.strip()
    if not line or line.startswith(("#", ";")): continue
    k, sep, v = line.partition("=")
    if not sep or k in values: raise SystemExit("Invalid or duplicate environment assignment")
    values[k] = v.strip().strip("'").strip('"')
if values.get("DATA_DIR") != data or "http://" + values.get("APP_ADDR", "") + "/login" != probe:
    raise SystemExit("DATA_DIR or APP_ADDR does not match this deployment")
if not values.get("APP_PUBLIC_URL") or values.get("COOKIE_SECURE") not in ("true","false"):
    raise SystemExit("Missing public URL or cookie configuration")
if bool(values.get("RELAY_URL")) != bool(values.get("RELAY_SECRET")):
    raise SystemExit("Incomplete Relay configuration")
PY
backup=$(mktemp -d /var/backups/site-observatory-update.XXXXXXXX)
cp -a "$bin" "$backup/binary"
cp -a "$envfile" "$backup/environment"
stage=$(mktemp /opt/site-observatory/.update.XXXXXXXX)
trap 'rm -f "$stage"' EXIT
install -m 755 -o root -g root "$candidate" "$stage"
printf '%s  %s\n' "$digest" "$stage" | sha256sum -c -
rollback() {
    trap - ERR INT TERM
    set +e
    echo "Update failed; restoring old binary. Backup: $backup" >&2
    systemctl stop "$unit"
    install -m 755 -o root -g root "$backup/binary" "$stage" && mv -f "$stage" "$bin" && systemctl start "$unit"
    # Do not automatically overwrite live data; schema rollback requires review.
    systemctl is-active "$unit"
    exit 1
}
trap rollback ERR INT TERM
systemctl stop "$unit"
# Offline snapshot includes WAL files and keys; preserve ownership/modes.
cp -a "$data" "$backup/data"
python3 - "$backup/data/observatory.db" <<'PY'
import sqlite3, sys
with sqlite3.connect(sys.argv[1]) as db:
    if db.execute("PRAGMA integrity_check").fetchone()[0] != "ok":
        raise SystemExit("Backup integrity check failed")
PY
mv -f "$stage" "$bin"
systemctl start "$unit"
# Require consecutive healthy checks, rather than accepting systemd's initial start.
for i in 1 2 3; do
    sleep 3
    systemctl is-active --quiet "$unit"
    curl --fail --silent --max-time 5 "$probe" >/dev/null
done
trap - ERR INT TERM
echo "Update successful. Backup: $backup"
