package storage

import (
 "context"
 "database/sql"
 "errors"
 "fmt"
 "strings"
 "time"

 "example.com/site-observatory/internal/robots"
)
type RobotsState struct {
 MonitorID int64
 MonitorName,SiteURL string
 Enabled bool
 Status,LastError,LastChecked,NextCheck string
 Content,SitemapList,RootBlocks string
}
type RobotsChange struct{
 At,Kind,Detail string
}
func(d *DB) MigrateRobots()error{
 _,err:=d.Exec(`
 CREATE TABLE IF NOT EXISTS robots_settings(
 monitor_id INTEGER PRIMARY KEY REFERENCES monitors(id) ON DELETE CASCADE,
 enabled INTEGER NOT NULL DEFAULT 0,
 status TEXT NOT NULL DEFAULT 'PENDING',
 last_error TEXT NOT NULL DEFAULT '',
 last_checked TEXT NOT NULL DEFAULT '',
 next_check TEXT NOT NULL DEFAULT '',
 content TEXT NOT NULL DEFAULT '',
 sitemap_list TEXT NOT NULL DEFAULT '',
 root_blocks TEXT NOT NULL DEFAULT '',
 has_baseline INTEGER NOT NULL DEFAULT 0
 );
 CREATE TABLE IF NOT EXISTS robots_changes(
 id INTEGER PRIMARY KEY,
 monitor_id INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
 checked_at TEXT NOT NULL,
 kind TEXT NOT NULL,
 detail TEXT NOT NULL
 );
 CREATE INDEX IF NOT EXISTS idx_robots_changes ON robots_changes(monitor_id,id DESC);
 INSERT OR IGNORE INTO robots_settings(monitor_id) SELECT id FROM monitors;
 `)
 return err
}
const robotsColumns=`m.id,m.name,m.url,COALESCE(r.enabled,0),COALESCE(r.status,'PENDING'),COALESCE(r.last_error,''),COALESCE(r.last_checked,''),COALESCE(r.next_check,''),COALESCE(r.content,''),COALESCE(r.sitemap_list,''),COALESCE(r.root_blocks,'')`
func scanRobots(scanner interface{Scan(...any)error})(RobotsState,error){
 var s RobotsState;var enabled int
 e:=scanner.Scan(&s.MonitorID,&s.MonitorName,&s.SiteURL,&enabled,&s.Status,&s.LastError,&s.LastChecked,&s.NextCheck,&s.Content,&s.SitemapList,&s.RootBlocks)
 s.Enabled=enabled==1
 return s,e
}
func(d *DB) RobotsList()([]RobotsState,error){
 rows,e:=d.Query(`SELECT `+robotsColumns+` FROM monitors m LEFT JOIN robots_settings r ON r.monitor_id=m.id ORDER BY m.name`)
 if e!=nil{return nil,e}
 defer rows.Close()
 list:=[]RobotsState{}
 for rows.Next(){s,err:=scanRobots(rows);if err!=nil{return nil,err};list=append(list,s)}
 return list,rows.Err()
}
func(d *DB) RobotsForMonitor(id int64)(RobotsState,error){
 return scanRobots(d.QueryRow(`SELECT `+robotsColumns+` FROM monitors m LEFT JOIN robots_settings r ON r.monitor_id=m.id WHERE m.id=?`,id))
}
func(d *DB) ConfigureRobots(id int64,enabled bool)error {
 var exists int
 if e:=d.QueryRow(`SELECT COUNT(*) FROM monitors WHERE id=?`,id).Scan(&exists);e!=nil{return e}
 if exists==0{return sql.ErrNoRows}
 _,e:=d.Exec(`INSERT INTO robots_settings(monitor_id,enabled,next_check) VALUES(?,?,'')
 ON CONFLICT(monitor_id) DO UPDATE SET enabled=excluded.enabled,next_check=''`,id,enabled)
 return e
}
func(d *DB) DueRobots(now time.Time,limit int)([]RobotsState,error){
 if limit<1||limit>20{limit=4}
 rows,e:=d.Query(`SELECT `+robotsColumns+` FROM monitors m JOIN robots_settings r ON r.monitor_id=m.id WHERE r.enabled=1 AND m.enabled=1 AND (r.next_check='' OR r.next_check<=?) ORDER BY r.next_check,m.id LIMIT ?`,now.UTC().Format(time.RFC3339),limit)
 if e!=nil{return nil,e}
 defer rows.Close()
 list:=[]RobotsState{}
 for rows.Next(){s,err:=scanRobots(rows);if err!=nil{return nil,err};list=append(list,s)}
 return list,rows.Err()
}
func (d *DB) RecordRobotsError(ctx context.Context,id int64,readErr error)error{
 tx,err:=d.BeginTx(ctx,nil);if err!=nil{return err}
 defer tx.Rollback()
 var previous,name string;var enabled int
 if err=tx.QueryRowContext(ctx,`SELECT r.status,m.name,r.enabled FROM robots_settings r JOIN monitors m ON m.id=r.monitor_id WHERE r.monitor_id=?`,id).Scan(&previous,&name,&enabled);err!=nil{return err}
 if enabled==0{return nil}
 now:=time.Now().UTC()
 reason:=readErr.Error()
 if len(reason)>450{reason=reason[:450]}
 _,err=tx.ExecContext(ctx,`UPDATE robots_settings SET status='ERROR',last_error=?,last_checked=?,next_check=? WHERE monitor_id=?`,reason,now.Format(time.RFC3339),now.Add(24*time.Hour).Format(time.RFC3339),id)
 if err!=nil{return err}
 if previous!="ERROR"{
  if err=enqueueSitemapAlert(ctx,tx,fmt.Sprintf("🔴 robots.txt недоступен: %s\nОшибка: %s",name,reason));err!=nil{return err}
  if err=insertRobotsChange(ctx,tx,id,"ERROR",reason,now);err!=nil{return err}
 }
 return tx.Commit()
}
func insertRobotsChange(ctx context.Context,tx *sql.Tx,id int64,kind,detail string,now time.Time)error{
 if len(detail)>1000{detail=detail[:1000]}
 _,err:=tx.ExecContext(ctx,`INSERT INTO robots_changes(monitor_id,checked_at,kind,detail) VALUES(?,?,?,?)`,id,now.Format(time.RFC3339),kind,detail)
 return err
}
func (d *DB) RecordRobots(ctx context.Context,id int64,source string)error{
 if len(source)>512<<10{return errors.New("robots content exceeds limit")}
 parsed:=robots.Parse(source)
 tx,err:=d.BeginTx(ctx,nil);if err!=nil{return err}
 defer tx.Rollback()
 var oldContent,oldStatus,name string;var enabled,baseline int
 err=tx.QueryRowContext(ctx,`SELECT r.content,r.status,r.has_baseline,r.enabled,m.name FROM robots_settings r JOIN monitors m ON m.id=r.monitor_id WHERE r.monitor_id=?`,id).Scan(&oldContent,&oldStatus,&baseline,&enabled,&name)
 if err!=nil{return err}
 if enabled==0{return nil}
 previous:=robots.Parse(oldContent)
 now:=time.Now().UTC()
 if baseline==1 && parsed.Hash!=previous.Hash {
  detail:="Содержимое robots.txt изменилось"
  if err=insertRobotsChange(ctx,tx,id,"CHANGED",detail,now);err!=nil{return err}
  if critical:=robots.AddedRootBlocks(previous,parsed);len(critical)>0 {
   text:=fmt.Sprintf("⚠️ Опасное правило в robots.txt: %s\nНовый Disallow: / для User-agent: %s\nПроверьте директивы Allow и правила индексации.",name,strings.Join(critical,", "))
   if err=enqueueSitemapAlert(ctx,tx,text);err!=nil{return err}
   if err=insertRobotsChange(ctx,tx,id,"ROOT_BLOCK",strings.Join(critical,", "),now);err!=nil{return err}
  }
  if robots.SitemapChanged(previous,parsed) {
   if err=insertRobotsChange(ctx,tx,id,"SITEMAP",fmt.Sprintf("Sitemap: %s → %s",strings.Join(previous.SitemapURLs,", "),strings.Join(parsed.SitemapURLs,", ")),now);err!=nil{return err}
   if err=enqueueSitemapAlert(ctx,tx,fmt.Sprintf("ℹ️ Изменились директивы Sitemap в robots.txt: %s",name));err!=nil{return err}
  }
 }
 if oldStatus=="ERROR" {
  if err=insertRobotsChange(ctx,tx,id,"RECOVERED","robots.txt снова доступен",now);err!=nil{return err}
  if err=enqueueSitemapAlert(ctx,tx,fmt.Sprintf("🟢 robots.txt снова доступен: %s",name));err!=nil{return err}
 }
 _,err=tx.ExecContext(ctx,`UPDATE robots_settings SET status='OK',last_error='',last_checked=?,next_check=?,content=?,sitemap_list=?,root_blocks=?,has_baseline=1 WHERE monitor_id=?`,
  now.Format(time.RFC3339),now.Add(24*time.Hour).Format(time.RFC3339),source,strings.Join(parsed.SitemapURLs,"\n"),strings.Join(parsed.RootBlocks,", "),id)
 if err!=nil{return err}
 _,err=tx.ExecContext(ctx,`DELETE FROM robots_changes WHERE monitor_id=? AND id NOT IN (SELECT id FROM robots_changes WHERE monitor_id=? ORDER BY id DESC LIMIT 100)`,id,id)
 if err!=nil{return err}
 return tx.Commit()
}
func(d *DB) RobotsChanges(id int64)([]RobotsChange,error){
 rows,e:=d.Query(`SELECT checked_at,kind,detail FROM robots_changes WHERE monitor_id=? ORDER BY id DESC LIMIT 100`,id)
 if e!=nil{return nil,e}
 defer rows.Close()
 list:=[]RobotsChange{}
 for rows.Next(){var v RobotsChange;if e=rows.Scan(&v.At,&v.Kind,&v.Detail);e!=nil{return nil,e};list=append(list,v)}
 return list,rows.Err()
}
