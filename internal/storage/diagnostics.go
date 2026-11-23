package storage

import (
 "context"
 "database/sql"
 "time"
)

type Diagnostic struct {
 MonitorID int64
 CheckedAt string
 TLSStatus string
 TLSDays int
 TLSExpiry string
 DNSSummary string
 DNSChanged bool
 Title string
 Canonical string
 Robots string
 SEOStatus string
 Error string
}
type UptimeStats struct {
 Total int
 Successful int
 AvgLatency int64
 Uptime float64
}
type GraphPoint struct {
 X int
 Y int
}
func (d *DB) MigrateDiagnostics() error {
 _,e:=d.Exec(`
 CREATE TABLE IF NOT EXISTS diagnostic_results (
  monitor_id INTEGER PRIMARY KEY REFERENCES monitors(id) ON DELETE CASCADE,
  checked_at TEXT NOT NULL,
  tls_status TEXT NOT NULL,
  tls_days INTEGER NOT NULL,
  tls_expiry TEXT NOT NULL,
  dns_summary TEXT NOT NULL,
  dns_changed INTEGER NOT NULL DEFAULT 0,
  title TEXT NOT NULL,
  canonical TEXT NOT NULL,
  robots TEXT NOT NULL,
  seo_status TEXT NOT NULL,
  error TEXT NOT NULL
 );`)
 return e
}
func (d *DB) SaveDiagnostic(ctx context.Context,r Diagnostic)error {
 _,e:=d.ExecContext(ctx,`
 INSERT INTO diagnostic_results(monitor_id,checked_at,tls_status,tls_days,tls_expiry,dns_summary,dns_changed,title,canonical,robots,seo_status,error)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
 ON CONFLICT(monitor_id) DO UPDATE SET
 checked_at=excluded.checked_at,tls_status=excluded.tls_status,tls_days=excluded.tls_days,
 tls_expiry=excluded.tls_expiry,dns_changed=CASE WHEN diagnostic_results.dns_summary<>'' AND diagnostic_results.dns_summary<>excluded.dns_summary THEN 1 ELSE 0 END,
 dns_summary=excluded.dns_summary,title=excluded.title,canonical=excluded.canonical,
 robots=excluded.robots,seo_status=excluded.seo_status,error=excluded.error`,
 r.MonitorID,r.CheckedAt,r.TLSStatus,r.TLSDays,r.TLSExpiry,r.DNSSummary,r.DNSChanged,r.Title,r.Canonical,r.Robots,r.SEOStatus,r.Error)
 return e
}
func (d *DB) Diagnostic(id int64)(Diagnostic,error){
 var r Diagnostic;var changed int
 err:=d.QueryRow(`SELECT monitor_id,checked_at,tls_status,tls_days,tls_expiry,dns_summary,dns_changed,title,canonical,robots,seo_status,error FROM diagnostic_results WHERE monitor_id=?`,id).
 Scan(&r.MonitorID,&r.CheckedAt,&r.TLSStatus,&r.TLSDays,&r.TLSExpiry,&r.DNSSummary,&changed,&r.Title,&r.Canonical,&r.Robots,&r.SEOStatus,&r.Error)
 r.DNSChanged=changed==1
 return r,err
}
func (d *DB) Uptime(id int64,since time.Time)(UptimeStats,error){
 var s UptimeStats
 e:=d.QueryRow(`SELECT COUNT(*),COALESCE(SUM(success),0),COALESCE(AVG(latency_ms),0) FROM checks WHERE monitor_id=? AND checked_at>=?`,id,since.UTC().Format(time.RFC3339)).
 Scan(&s.Total,&s.Successful,&s.AvgLatency)
 if s.Total>0 {s.Uptime=100*float64(s.Successful)/float64(s.Total)}
 return s,e
}
func (d *DB) LastLatencies(id int64,limit int)([]int64,error){
 rows,e:=d.Query(`SELECT latency_ms FROM (SELECT latency_ms,checked_at,id FROM checks WHERE monitor_id=? ORDER BY checked_at DESC,id DESC LIMIT ?) ORDER BY checked_at ASC,id ASC`,id,limit)
 if e!=nil{return nil,e}
 defer rows.Close()
 var list []int64
 for rows.Next(){var n int64;if e=rows.Scan(&n);e!=nil{return nil,e};list=append(list,n)}
 return list,rows.Err()
}
var _=sql.ErrNoRows
