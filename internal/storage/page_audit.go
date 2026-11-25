package storage

import (
 "context"
 "database/sql"
 "errors"
 "time"
)

type PageAuditConfig struct{
 MonitorID int64
 MonitorName,SiteURL string
 Enabled bool
 Total,Checked,Errors,Noindex int
}
type DuePage struct {
 MonitorID int64
 SiteURL,URL string
}
type PageAuditResult struct{
 URL,CheckedAt,Title,Canonical,Robots,Error string
 Status int
 Noindex bool
}
type PageAuditEvent struct{
 CheckedAt,URL,Kind,Detail string
}
func(d *DB) MigratePageAudit()error{
 _,err:=d.Exec(`
 CREATE TABLE IF NOT EXISTS page_audit_settings(
 monitor_id INTEGER PRIMARY KEY REFERENCES monitors(id) ON DELETE CASCADE,
 enabled INTEGER NOT NULL DEFAULT 0
 );
 CREATE TABLE IF NOT EXISTS page_audit_results(
 monitor_id INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
 url TEXT NOT NULL,
 checked_at TEXT NOT NULL,
 http_status INTEGER NOT NULL,
 title TEXT NOT NULL DEFAULT '',
 canonical TEXT NOT NULL DEFAULT '',
 robots TEXT NOT NULL DEFAULT '',
 noindex INTEGER NOT NULL DEFAULT 0,
 last_error TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(monitor_id,url)
 );
 CREATE INDEX IF NOT EXISTS idx_page_audit_due ON page_audit_results(monitor_id,checked_at);
 CREATE TABLE IF NOT EXISTS page_audit_events(
 id INTEGER PRIMARY KEY,
 monitor_id INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
 url TEXT NOT NULL,
 checked_at TEXT NOT NULL,
 kind TEXT NOT NULL,
 detail TEXT NOT NULL
 );
 CREATE INDEX IF NOT EXISTS idx_page_audit_events ON page_audit_events(monitor_id,id DESC);
 `)
 return err
}
func (d *DB) PageAuditConfigs()([]PageAuditConfig,error){
 rows,err:=d.Query(`SELECT m.id,m.name,m.url,COALESCE(a.enabled,0),
 (SELECT COUNT(*) FROM sitemap_urls su WHERE su.monitor_id=m.id),
 (SELECT COUNT(*) FROM page_audit_results pr JOIN sitemap_urls su ON su.monitor_id=pr.monitor_id AND su.url=pr.url WHERE pr.monitor_id=m.id),
 (SELECT COUNT(*) FROM page_audit_results pr JOIN sitemap_urls su ON su.monitor_id=pr.monitor_id AND su.url=pr.url WHERE pr.monitor_id=m.id AND (pr.http_status>=400 OR pr.http_status=0 OR pr.last_error<>'')),
 (SELECT COUNT(*) FROM page_audit_results pr JOIN sitemap_urls su ON su.monitor_id=pr.monitor_id AND su.url=pr.url WHERE pr.monitor_id=m.id AND pr.noindex=1)
 FROM monitors m LEFT JOIN page_audit_settings a ON a.monitor_id=m.id ORDER BY m.name`)
 if err!=nil{return nil,err}
 defer rows.Close()
 out:=[]PageAuditConfig{}
 for rows.Next(){
  var v PageAuditConfig;var enabled int
  if err=rows.Scan(&v.MonitorID,&v.MonitorName,&v.SiteURL,&enabled,&v.Total,&v.Checked,&v.Errors,&v.Noindex);err!=nil{return nil,err}
  v.Enabled=enabled==1;out=append(out,v)
 }
 return out,rows.Err()
}
func(d *DB) ConfigurePageAudit(id int64,enabled bool)error{
 var exists int
 if err:=d.QueryRow(`SELECT COUNT(*) FROM monitors WHERE id=?`,id).Scan(&exists);err!=nil{return err}
 if exists==0{return sql.ErrNoRows}
 _,err:=d.Exec(`INSERT INTO page_audit_settings(monitor_id,enabled) VALUES(?,?)
 ON CONFLICT(monitor_id) DO UPDATE SET enabled=excluded.enabled`,id,enabled)
 return err
}
func(d *DB) PageAuditDue(now time.Time,limit int)([]DuePage,error){
 if limit<1||limit>10{limit=5}
 cutoff:=now.UTC().Add(-24*time.Hour).Format(time.RFC3339)
 rows,err:=d.Query(`SELECT m.id,m.url,su.url
 FROM sitemap_urls su JOIN monitors m ON m.id=su.monitor_id
 JOIN sitemap_settings sm ON sm.monitor_id=m.id
 JOIN page_audit_settings a ON a.monitor_id=m.id
 LEFT JOIN page_audit_results p ON p.monitor_id=su.monitor_id AND p.url=su.url
 WHERE a.enabled=1 AND m.enabled=1 AND sm.enabled=1 AND sm.has_baseline=1
 AND (p.checked_at IS NULL OR p.checked_at<=?)
 ORDER BY p.checked_at ASC,su.monitor_id,su.url LIMIT ?`,cutoff,limit)
 if err!=nil{return nil,err}
 defer rows.Close()
 out:=[]DuePage{}
 for rows.Next(){var v DuePage;if err=rows.Scan(&v.MonitorID,&v.SiteURL,&v.URL);err!=nil{return nil,err};out=append(out,v)}
 return out,rows.Err()
}
func(d *DB) SavePageAudit(ctx context.Context,id int64,v PageAuditResult)error{
 tx,err:=d.BeginTx(ctx,nil);if err!=nil{return err}
 defer tx.Rollback()
 var old PageAuditResult;var oldNoindex int
 err=tx.QueryRowContext(ctx,`SELECT http_status,title,canonical,robots,noindex,last_error FROM page_audit_results WHERE monitor_id=? AND url=?`,id,v.URL).Scan(&old.Status,&old.Title,&old.Canonical,&old.Robots,&oldNoindex,&old.Error)
 baseline:=errors.Is(err,sql.ErrNoRows)
 if err!=nil&&!baseline{return err}
 old.Noindex=oldNoindex==1
 now:=time.Now().UTC().Format(time.RFC3339)
 if baseline {v.CheckedAt=now} else {v.CheckedAt=now}
 _,err=tx.ExecContext(ctx,`INSERT INTO page_audit_results(monitor_id,url,checked_at,http_status,title,canonical,robots,noindex,last_error)
 VALUES(?,?,?,?,?,?,?,?,?)
 ON CONFLICT(monitor_id,url) DO UPDATE SET checked_at=excluded.checked_at,http_status=excluded.http_status,title=excluded.title,canonical=excluded.canonical,robots=excluded.robots,noindex=excluded.noindex,last_error=excluded.last_error`,
 id,v.URL,now,v.Status,v.Title,v.Canonical,v.Robots,v.Noindex,v.Error)
 if err!=nil{return err}
 if !baseline {
  events:=[][2]string{}
  if old.Status!=v.Status{events=append(events,[2]string{"HTTP","HTTP status changed"})}
  if old.Title!=v.Title{events=append(events,[2]string{"TITLE","Title changed"})}
  if old.Canonical!=v.Canonical{events=append(events,[2]string{"CANONICAL","Canonical changed"})}
  if old.Noindex!=v.Noindex{events=append(events,[2]string{"NOINDEX","Noindex changed"})}
  if old.Robots!=v.Robots{events=append(events,[2]string{"ROBOTS","Robots directives changed"})}
  for _,event:=range events{
   if _,err=tx.ExecContext(ctx,`INSERT INTO page_audit_events(monitor_id,url,checked_at,kind,detail) VALUES(?,?,?,?,?)`,id,v.URL,now,event[0],event[1]);err!=nil{return err}
  }
 }
 _,err=tx.ExecContext(ctx,`DELETE FROM page_audit_events WHERE monitor_id=? AND id NOT IN (SELECT id FROM page_audit_events WHERE monitor_id=? ORDER BY id DESC LIMIT 500)`,id,id)
 if err!=nil{return err}
 return tx.Commit()
}
func(d *DB) PageAuditResults(id int64,limit int)([]PageAuditResult,error){
 if limit<1||limit>500{limit=100}
 rows,err:=d.Query(`SELECT p.url,p.checked_at,p.http_status,p.title,p.canonical,p.robots,p.noindex,p.last_error
 FROM page_audit_results p JOIN sitemap_urls s ON p.monitor_id=s.monitor_id AND p.url=s.url
 WHERE p.monitor_id=? ORDER BY (p.http_status>=400 OR p.last_error<>'' OR p.noindex=1) DESC,p.checked_at DESC LIMIT ?`,id,limit)
 if err!=nil{return nil,err}
 defer rows.Close()
 out:=[]PageAuditResult{}
 for rows.Next(){var v PageAuditResult;var noindex int;if err=rows.Scan(&v.URL,&v.CheckedAt,&v.Status,&v.Title,&v.Canonical,&v.Robots,&noindex,&v.Error);err!=nil{return nil,err};v.Noindex=noindex!=0;out=append(out,v)}
 return out,rows.Err()
}
func(d *DB) PageAuditEvents(id int64)([]PageAuditEvent,error){
 rows,err:=d.Query(`SELECT checked_at,url,kind,detail FROM page_audit_events WHERE monitor_id=? ORDER BY id DESC LIMIT 100`,id)
 if err!=nil{return nil,err}
 defer rows.Close()
 out:=[]PageAuditEvent{}
 for rows.Next(){var v PageAuditEvent;if err=rows.Scan(&v.CheckedAt,&v.URL,&v.Kind,&v.Detail);err!=nil{return nil,err};out=append(out,v)}
 return out,rows.Err()
}
