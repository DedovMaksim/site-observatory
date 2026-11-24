package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	_ "modernc.org/sqlite"
	"time"
)

type DB struct{ *sql.DB }
type Monitor struct {
	ID                                     int64
	Name, URL, Status, LastError           string
	Interval, Timeout, Failures, Successes int
	Enabled                                bool
	LastCode                               int
	LastLatency                            int64
	LastChecked                            string
}
type Check struct {
	ID, MonitorID int64
	CheckedAt     string
	Code          int
	Latency       int64
	Success       bool
	Error         string
}
type Incident struct {
	ID, MonitorID              int64
	StartedAt, EndedAt, Reason string
}

func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON;`); err != nil {
		db.Close()
		return nil, err
	}
	s := &DB{db}
	if err := s.Migrate(); err != nil { db.Close(); return nil,err }
 if err := s.MigrateTelegram(); err != nil { db.Close(); return nil,err }
 if err := s.MigrateDiagnostics(); err != nil { db.Close(); return nil,err }
 if err := s.MigrateDomains(); err != nil { db.Close(); return nil,err }
 if err := s.ReconcileDomains(); err != nil { db.Close(); return nil,err }
 return s,nil
}
func (d *DB) Migrate() error {
	_, err := d.Exec(`
CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, username TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS monitors (id INTEGER PRIMARY KEY, name TEXT NOT NULL, url TEXT NOT NULL, interval_sec INTEGER NOT NULL DEFAULT 60, timeout_sec INTEGER NOT NULL DEFAULT 10, enabled INTEGER NOT NULL DEFAULT 1, status TEXT NOT NULL DEFAULT 'UNKNOWN', failures INTEGER NOT NULL DEFAULT 0, successes INTEGER NOT NULL DEFAULT 0, last_code INTEGER NOT NULL DEFAULT 0, last_latency INTEGER NOT NULL DEFAULT 0, last_error TEXT NOT NULL DEFAULT '', last_checked TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS checks (id INTEGER PRIMARY KEY, monitor_id INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE, checked_at TEXT NOT NULL, code INTEGER NOT NULL DEFAULT 0, latency_ms INTEGER NOT NULL DEFAULT 0, success INTEGER NOT NULL, error TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS idx_checks_monitor_time ON checks(monitor_id,checked_at DESC);
CREATE TABLE IF NOT EXISTS incidents (id INTEGER PRIMARY KEY, monitor_id INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE, started_at TEXT NOT NULL, ended_at TEXT NOT NULL DEFAULT '', reason TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS idx_incidents_monitor ON incidents(monitor_id,started_at DESC);
`)
	return err
}
func (d *DB) UserHash() (string, error) {
	var h string
	err := d.QueryRow(`SELECT password_hash FROM users WHERE username='admin'`).Scan(&h)
	return h, err
}
func (d *DB) CreateAdmin(hash string) error {
	_, err := d.Exec(`INSERT INTO users(username,password_hash) VALUES('admin',?)`, hash)
	return err
}
func scanMon(rows *sql.Rows) (Monitor, error) {
	var m Monitor
	var e int
	err := rows.Scan(&m.ID, &m.Name, &m.URL, &m.Interval, &m.Timeout, &e, &m.Status, &m.Failures, &m.Successes, &m.LastCode, &m.LastLatency, &m.LastError, &m.LastChecked)
	m.Enabled = e == 1
	return m, err
}

const cols = `id,name,url,interval_sec,timeout_sec,enabled,status,failures,successes,last_code,last_latency,last_error,last_checked`

func (d *DB) Monitors() ([]Monitor, error) {
	rows, err := d.Query(`SELECT ` + cols + ` FROM monitors ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Monitor{}
	for rows.Next() {
		m, e := scanMon(rows)
		if e != nil {
			return nil, e
		}
		list = append(list, m)
	}
	return list, rows.Err()
}
func (d *DB) Monitor(id int64) (Monitor, error) {
	rows, err := d.Query(`SELECT `+cols+` FROM monitors WHERE id=?`, id)
	if err != nil {
		return Monitor{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return Monitor{}, sql.ErrNoRows
	}
	return scanMon(rows)
}
func (d *DB) CreateMonitor(name, url string, interval, timeout int, enabled bool) (int64, error) {
	r, e := d.Exec(`INSERT INTO monitors(name,url,interval_sec,timeout_sec,enabled) VALUES(?,?,?,?,?)`, name, url, interval, timeout, enabled)
	if e != nil {
		return 0, e
	}
	return r.LastInsertId()
}
func (d *DB) UpdateMonitor(id int64, name, url string, interval, timeout int, enabled bool) error {
	_, e := d.Exec(`UPDATE monitors SET name=?,url=?,interval_sec=?,timeout_sec=?,enabled=?,status='UNKNOWN',failures=0,successes=0 WHERE id=?`, name, url, interval, timeout, enabled, id)
	return e
}
func (d *DB) DeleteMonitor(id int64) error {
	_, e := d.Exec(`DELETE FROM monitors WHERE id=?`, id)
	return e
}
func (d *DB) Checks(id int64, limit int) ([]Check, error) {
	rows, e := d.Query(`SELECT id,monitor_id,checked_at,code,latency_ms,success,error FROM checks WHERE monitor_id=? ORDER BY checked_at DESC LIMIT ?`, id, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Check{}
	for rows.Next() {
		var c Check
		var v int
		e = rows.Scan(&c.ID, &c.MonitorID, &c.CheckedAt, &c.Code, &c.Latency, &v, &c.Error)
		if e != nil {
			return nil, e
		}
		c.Success = v == 1
		out = append(out, c)
	}
	return out, rows.Err()
}
func (d *DB) Incidents(id int64, limit int) ([]Incident, error) {
	rows, e := d.Query(`SELECT id,monitor_id,started_at,ended_at,reason FROM incidents WHERE (?=0 OR monitor_id=?) ORDER BY started_at DESC LIMIT ?`, id, id, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Incident{}
	for rows.Next() {
		var x Incident
		if e = rows.Scan(&x.ID, &x.MonitorID, &x.StartedAt, &x.EndedAt, &x.Reason); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// Record atomically updates the consecutive check counters and incident lifecycle.
func (d *DB) Record(ctx context.Context, id int64, c Check) error {
	tx, e := d.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var status, name, targetURL string
	var failures, successes int
	e = tx.QueryRowContext(ctx, `SELECT status,failures,successes,name,url FROM monitors WHERE id=?`, id).Scan(&status, &failures, &successes, &name, &targetURL)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	ok := 0
	if c.Success {
		ok = 1
		failures = 0
		successes++
	} else {
		failures++
		successes = 0
	}
	next := status
	if !c.Success && failures >= 3 {
		next = "DOWN"
	}
	if c.Success && successes >= 2 {
		next = "UP"
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO checks(monitor_id,checked_at,code,latency_ms,success,error) VALUES(?,?,?,?,?,?)`, id, c.CheckedAt, c.Code, c.Latency, ok, c.Error); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE monitors SET status=?,failures=?,successes=?,last_code=?,last_latency=?,last_error=?,last_checked=? WHERE id=?`, next, failures, successes, c.Code, c.Latency, c.Error, c.CheckedAt, id); e != nil {
		return e
	}
	if next == "DOWN" && status != "DOWN" {
        msg := fmt.Sprintf("🔴 Сайт недоступен: %s\n%s\nПричина: %s\nВремя (UTC): %s",name,targetURL,c.Error,c.CheckedAt)
        if c.Error=="" {msg=fmt.Sprintf("🔴 Сайт недоступен: %s\n%s\nHTTP: %d\nВремя (UTC): %s",name,targetURL,c.Code,c.CheckedAt)}
        if e = EnqueueTelegram(ctx,tx,"DOWN",msg); e != nil { return e }
		_, e = tx.ExecContext(ctx, `INSERT INTO incidents(monitor_id,started_at,reason) VALUES(?,?,?)`, id, c.CheckedAt, c.Error)
		if e != nil {
			return e
		}
	}
	if next == "UP" && status == "DOWN" {
        msg:=fmt.Sprintf("🟢 Сайт восстановлен: %s\n%s\nHTTP: %d\nВремя (UTC): %s",name,targetURL,c.Code,c.CheckedAt)
        if e = EnqueueTelegram(ctx,tx,"UP",msg); e != nil { return e }
		_, e = tx.ExecContext(ctx, `UPDATE incidents SET ended_at=? WHERE monitor_id=? AND ended_at=''`, c.CheckedAt, id)
		if e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (d *DB) Prune(ctx context.Context) error {
	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	r, e := d.ExecContext(ctx, `DELETE FROM checks WHERE checked_at < ?`, cutoff)
	if e != nil {
		return e
	}
	_, e = d.ExecContext(ctx, `PRAGMA wal_checkpoint(PASSIVE)`)
	if e != nil {
		return fmt.Errorf("prune deleted %v rows but checkpoint failed: %w", r, e)
	}
	return nil
}
