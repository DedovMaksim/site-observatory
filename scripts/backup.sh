#!/usr/bin/env bash
set -euo pipefail
# Consistent online SQLite snapshot + encrypted Telegram key backup.
DB_PATH="${DB_PATH:-./data/observatory.db}"
BACKUP_DIR="${BACKUP_DIR:-./backups}"
mkdir -p "$BACKUP_DIR"
python3 - "$DB_PATH" "$BACKUP_DIR" <<'PYBACKUP'
import datetime
import pathlib
import shutil
import sqlite3
import sys

source = pathlib.Path(sys.argv[1])
directory = pathlib.Path(sys.argv[2])
stamp = datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
snapshot = directory / f'observatory-{stamp}.db'
if not source.exists():
    raise SystemExit('Database missing: ' + str(source))
with sqlite3.connect(str(source)) as src, sqlite3.connect(str(snapshot)) as dst:
    src.backup(dst)
snapshot.chmod(0o600)
print(snapshot)
key = source.parent / 'telegram.key'
if key.exists():
    destination = directory / f'telegram-{stamp}.key'
    shutil.copyfile(key, destination)
    destination.chmod(0o600)
    print(destination)
    print('Keep the .db and matching .key together, protected from public access.')
PYBACKUP
