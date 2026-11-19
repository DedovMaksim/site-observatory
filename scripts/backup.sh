#!/usr/bin/env bash
set -euo pipefail
# Uses Python's stdlib SQLite backup API for an online-safe snapshot.
DB_PATH="${DB_PATH:-./data/observatory.db}"
BACKUP_DIR="${BACKUP_DIR:-./backups}"
mkdir -p "$BACKUP_DIR"
python3 - "$DB_PATH" "$BACKUP_DIR" <<'PYBACKUP'
import sqlite3,sys,datetime,pathlib
source=pathlib.Path(sys.argv[1]);dest=pathlib.Path(sys.argv[2])/('observatory-'+datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')+'.db')
if not source.exists(): raise SystemExit('Database missing: '+str(source))
with sqlite3.connect(str(source)) as src,sqlite3.connect(str(dest)) as dst: src.backup(dst)
print(dest)
PYBACKUP
