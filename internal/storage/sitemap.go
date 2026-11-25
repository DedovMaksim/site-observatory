package storage

import (
 "context"
 "database/sql"
 "errors"
 "time"
)

type SitemapState struct {
 MonitorID int64
 MonitorName string
 SiteURL string
 Enabled bool
 URL string
 Status string
 Error string
 LastChecked string
 NextCheck string
 Total int
 Added int
 Removed int
 Files int
}
type SitemapChange struct{
 URL string
 Kind string
 CheckedAt string
}
func (d *DB) MigrateSitemap() error{
 _,err:=d.Exec(`
 CREATE TABLE IF NOT EXISTS sitemap_settings (
 monitor_id INTEGER PRIMARY KEY REFERENCES monitors(id) ON DELETE CASCADE,
 enabled INTEGER NOT NULL DEFAULT 0,
 sitemap_url TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL DEFAULT 'PENDING',
 last_error TEXT NOT NULL DEFAULT '',
 last_checked TEXT NOT NULL DEFAULT '',
 next_check TEXT NOT NULL DEFAULT '',
 total INTEGER NOT NULL DEFAULT 0,
 added INTEGER NOT NULL DEFAULT 0,
 removed INTEGER NOT NULL DEFAULT 0,
 files INTEGER NOT NULL DEFAULT 0,
 has_baseline INTEGER NOT NULL DEFAULT 0
 );
 CREATE TABLE IF NOT EXISTS sitemap_baseline_sources (
 monitor_id INTEGER PRIMARY KEY REFERENCES monitors(id) ON DELETE CASCADE,
 source_url TEXT NOT NULL
 );
 CREATE TABLE IF NOT EXISTS sitemap_urls (
 monitor_id INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
 url TEXT NOT NULL,
 PRIMARY KEY(monitor_id,url)
 );
 CREATE TABLE IF NOT EXISTS sitemap_changes (
 id INTEGER PRIMARY KEY,
 monitor_id INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
 url TEXT NOT NULL,
 kind TEXT NOT NULL,
 checked_at TEXT NOT NULL
 );
 CREATE INDEX IF NOT EXISTS idx_sitemap_changes_recent ON sitemap_changes(monitor_id,id DESC);
 INSERT OR IGNORE INTO sitemap_settings(monitor_id) SELECT id FROM monitors;
 `)
 return err
}
const sitemapColumns=`m.id,m.name,m.url,COALESCE(s.enabled,0),COALESCE(s.sitemap_url,''),COALESCE(s.status,'PENDING'),COALESCE(s.last_error,''),COALESCE(s.last_checked,''),COALESCE(s.next_check,''),COALESCE(s.total,0),COALESCE(s.added,0),COALESCE(s.removed,0),COALESCE(s.files,0)`
func scanSitemap(scanner interface{Scan(...any)error})(SitemapState,error){
 var v SitemapState;var enabled int
 err:=scanner.Scan(&v.MonitorID,&v.MonitorName,&v.SiteURL,&enabled,&v.URL,&v.Status,&v.Error,&v.LastChecked,&v.NextCheck,&v.Total,&v.Added,&v.Removed,&v.Files)
 v.Enabled=enabled!=0
 return v,err
}
func (d *DB) SitemapList()([]SitemapState,error){
 rows,err:=d.Query(`SELECT `+sitemapColumns+` FROM monitors m LEFT JOIN sitemap_settings s ON s.monitor_id=m.id ORDER BY m.name`)
 if err!=nil{return nil,err}
 defer rows.Close()
 out:=[]SitemapState{}
 for rows.Next(){s,e:=scanSitemap(rows);if e!=nil{return nil,e};out=append(out,s)}
 return out,rows.Err()
}
func (d *DB) SitemapForMonitor(id int64)(SitemapState,error){
 row:=d.QueryRow(`SELECT `+sitemapColumns+` FROM monitors m LEFT JOIN sitemap_settings s ON s.monitor_id=m.id WHERE m.id=?`,id)
 return scanSitemap(row)
}
func (d *DB) ConfigureSitemap(id int64,enabled bool,raw string)error{
 var exists int
 if err:=d.QueryRow(`SELECT COUNT(*) FROM monitors WHERE id=?`,id).Scan(&exists);err!=nil{return err}
 if exists==0{return sql.ErrNoRows}
 _,err:=d.Exec(`INSERT INTO sitemap_settings(monitor_id,enabled,sitemap_url,status,next_check) VALUES(?,?,?,'PENDING','')
 ON CONFLICT(monitor_id) DO UPDATE SET enabled=excluded.enabled,sitemap_url=excluded.sitemap_url,status='PENDING',next_check=''`,id,enabled,raw)
 return err
}
func (d *DB) DueSitemaps(now time.Time,limit int)([]SitemapState,error){
 if limit<=0||limit>20{limit=5}
 rows,err:=d.Query(`SELECT `+sitemapColumns+` FROM monitors m JOIN sitemap_settings s ON s.monitor_id=m.id WHERE s.enabled=1 AND m.enabled=1 AND (s.next_check='' OR s.next_check<=?) ORDER BY s.next_check,m.id LIMIT ?`,now.UTC().Format(time.RFC3339),limit)
 if err!=nil{return nil,err}
 defer rows.Close()
 out:=[]SitemapState{}
 for rows.Next(){v,e:=scanSitemap(rows);if e!=nil{return nil,e};out=append(out,v)}
 return out,rows.Err()
}
func (d *DB) RecordSitemapError(ctx context.Context,id int64,scanErr error)error{
 msg:=scanErr.Error();if len(msg)>600{msg=msg[:600]}
 now:=time.Now().UTC()
 _,err:=d.ExecContext(ctx,`UPDATE sitemap_settings SET status='ERROR',last_error=?,last_checked=?,next_check=? WHERE monitor_id=?`,msg,now.Format(time.RFC3339),now.Add(24*time.Hour).Format(time.RFC3339),id)
 return err
}
func (d *DB) RecordSitemap(ctx context.Context,id int64,urls []string,files int)error{
 if len(urls)>20000{return errors.New("URL list too large")}
 tx,err:=d.BeginTx(ctx,nil);if err!=nil{return err}
 defer tx.Rollback()
 old:=map[string]bool{}
 rows,err:=tx.QueryContext(ctx,`SELECT url FROM sitemap_urls WHERE monitor_id=?`,id)
 if err!=nil{return err}
 for rows.Next(){var u string;if err=rows.Scan(&u);err!=nil{rows.Close();return err};old[u]=true}
 if err=rows.Err();err!=nil{rows.Close();return err}
 rows.Close()
 now:=time.Now().UTC()
 checked:=now.Format(time.RFC3339)
 // The initial scan establishes a baseline; it does not represent a burst
 // of newly published pages.
 var baseline int
 var sourceURL string
 err=tx.QueryRowContext(ctx,`SELECT has_baseline,sitemap_url FROM sitemap_settings WHERE monitor_id=?`,id).Scan(&baseline,&sourceURL)
 if err!=nil{return err}
 var previousSource string
 err=tx.QueryRowContext(ctx,`SELECT source_url FROM sitemap_baseline_sources WHERE monitor_id=?`,id).Scan(&previousSource)
 if err!=nil && !errors.Is(err,sql.ErrNoRows){return err}
 // First successful scan after a source switch establishes a fresh baseline.
 // Keep existing change history, but don't label newly covered URLs as added.
 initial:=baseline==0 || previousSource!=sourceURL
 seen:=map[string]bool{}
 added,removed:=0,0
 for _,u:=range urls{
  if seen[u]{continue}
  seen[u]=true
  if !old[u]{
   if !initial{
    added++
    if _,err=tx.ExecContext(ctx,`INSERT INTO sitemap_changes(monitor_id,url,kind,checked_at) VALUES(?,?,'ADDED',?)`,id,u,checked);err!=nil{return err}
   }
   if _,err=tx.ExecContext(ctx,`INSERT INTO sitemap_urls(monitor_id,url) VALUES(?,?)`,id,u);err!=nil{return err}
  }
 }
 for u:=range old{
  if !seen[u]{
   removed++
   if !initial{
    if _,err=tx.ExecContext(ctx,`INSERT INTO sitemap_changes(monitor_id,url,kind,checked_at) VALUES(?,?,'REMOVED',?)`,id,u,checked);err!=nil{return err}
   }
   if _,err=tx.ExecContext(ctx,`DELETE FROM sitemap_urls WHERE monitor_id=? AND url=?`,id,u);err!=nil{return err}
  }
 }
 if initial{added=0;removed=0}
 _,err=tx.ExecContext(ctx,`INSERT INTO sitemap_baseline_sources(monitor_id,source_url) VALUES(?,?) ON CONFLICT(monitor_id) DO UPDATE SET source_url=excluded.source_url`,id,sourceURL)
 if err!=nil{return err}
 _,err=tx.ExecContext(ctx,`UPDATE sitemap_settings SET status='OK',last_error='',last_checked=?,next_check=?,total=?,added=?,removed=?,files=?,has_baseline=1 WHERE monitor_id=?`,checked,now.Add(24*time.Hour).Format(time.RFC3339),len(seen),added,removed,files,id)
 if err!=nil{return err}
 // Bound retained change history per monitor.
 _,err=tx.ExecContext(ctx,`DELETE FROM sitemap_changes WHERE monitor_id=? AND id NOT IN (SELECT id FROM sitemap_changes WHERE monitor_id=? ORDER BY id DESC LIMIT 500)`,id,id)
 if err!=nil{return err}
 return tx.Commit()
}
func (d *DB) SitemapChanges(id int64,limit int)([]SitemapChange,error){
 if limit<=0||limit>100{limit=50}
 rows,err:=d.Query(`SELECT url,kind,checked_at FROM sitemap_changes WHERE monitor_id=? ORDER BY id DESC LIMIT ?`,id,limit)
 if err!=nil{return nil,err}
 defer rows.Close()
 out:=[]SitemapChange{}
 for rows.Next(){var v SitemapChange;if err=rows.Scan(&v.URL,&v.Kind,&v.CheckedAt);err!=nil{return nil,err};out=append(out,v)}
 return out,rows.Err()
}
