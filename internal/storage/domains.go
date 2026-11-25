package storage

import (
 "context"
 "database/sql"
 "errors"
 "fmt"
 "strings"
 "time"

 "example.com/site-observatory/internal/domains"
)
type DomainExpiry struct {
 Domain string
 ManualDate string
 RDAPDate string
 RDAPSource string
 LastChecked string
 NextCheck string
 LastError string
 MonitorCount int
 EffectiveDate string
 Source string
 DaysLeft int
 HasDate bool
}
func (d *DomainExpiry) Calculate(now time.Time) {
 d.EffectiveDate=d.RDAPDate
 d.Source=d.RDAPSource
 if d.ManualDate!="" {d.EffectiveDate=d.ManualDate;d.Source="Вручную"}
 if d.EffectiveDate==""{d.HasDate=false;return}
 expiry,err:=time.Parse("2006-01-02",d.EffectiveDate)
 if err!=nil{d.HasDate=false;return}
 d.HasDate=true
 d.DaysLeft=int(expiry.Sub(now.UTC().Truncate(24*time.Hour)).Hours()/24)
}
func (d *DB) MigrateDomains()error{
 _,err:=d.Exec(`
 CREATE TABLE IF NOT EXISTS domain_expiry (
 domain TEXT PRIMARY KEY,
 manual_date TEXT NOT NULL DEFAULT '',
 rdap_date TEXT NOT NULL DEFAULT '',
 rdap_source TEXT NOT NULL DEFAULT '',
 last_checked TEXT NOT NULL DEFAULT '',
 next_check TEXT NOT NULL DEFAULT '',
 last_error TEXT NOT NULL DEFAULT ''
 );
 CREATE TABLE IF NOT EXISTS monitor_domains (
 monitor_id INTEGER PRIMARY KEY REFERENCES monitors(id) ON DELETE CASCADE,
 domain TEXT NOT NULL REFERENCES domain_expiry(domain)
 );
 CREATE INDEX IF NOT EXISTS idx_monitor_domains_domain ON monitor_domains(domain);
 CREATE TABLE IF NOT EXISTS domain_alerts (
 domain TEXT NOT NULL REFERENCES domain_expiry(domain) ON DELETE CASCADE,
 expiry_date TEXT NOT NULL,
 level TEXT NOT NULL,
 queued_at TEXT NOT NULL,
 PRIMARY KEY(domain,expiry_date,level)
 );
 `)
 return err
}
func (d *DB) SyncMonitorDomain(id int64,raw string)error{
 domain,err:=domains.RegisteredDomain(raw)
 if err!=nil{
  _,e:=d.Exec(`DELETE FROM monitor_domains WHERE monitor_id=?`,id)
  return e
 }
 tx,err:=d.Begin();if err!=nil{return err}
 defer tx.Rollback()
 if _,err=tx.Exec(`INSERT OR IGNORE INTO domain_expiry(domain) VALUES(?)`,domain);err!=nil{return err}
 if _,err=tx.Exec(`INSERT INTO monitor_domains(monitor_id,domain) VALUES(?,?) ON CONFLICT(monitor_id) DO UPDATE SET domain=excluded.domain`,id,domain);err!=nil{return err}
 return tx.Commit()
}
func (d *DB) ReconcileDomains()error{
 monitors,err:=d.Monitors();if err!=nil{return err}
 for _,m:=range monitors {if err=d.SyncMonitorDomain(m.ID,m.URL);err!=nil{return err}}
 return nil
}
const domainColumns=`d.domain,d.manual_date,d.rdap_date,d.rdap_source,d.last_checked,d.next_check,d.last_error,COUNT(md.monitor_id)`
func scanDomain(rows *sql.Rows)(DomainExpiry,error){
 var d DomainExpiry
 e:=rows.Scan(&d.Domain,&d.ManualDate,&d.RDAPDate,&d.RDAPSource,&d.LastChecked,&d.NextCheck,&d.LastError,&d.MonitorCount)
 d.Calculate(time.Now())
 return d,e
}
func (d *DB) Domains()([]DomainExpiry,error){
 rows,err:=d.Query(`SELECT `+domainColumns+` FROM domain_expiry d JOIN monitor_domains md ON md.domain=d.domain GROUP BY d.domain ORDER BY d.domain`)
 if err!=nil{return nil,err}
 defer rows.Close()
 out:=[]DomainExpiry{}
 for rows.Next(){v,e:=scanDomain(rows);if e!=nil{return nil,e};out=append(out,v)}
 return out,rows.Err()
}
func (d *DB) DomainForMonitor(id int64)(DomainExpiry,error){
 rows,err:=d.Query(`SELECT `+domainColumns+` FROM domain_expiry d JOIN monitor_domains md ON md.domain=d.domain WHERE md.monitor_id=? GROUP BY d.domain`,id)
 if err!=nil{return DomainExpiry{},err}
 defer rows.Close()
 if !rows.Next(){return DomainExpiry{},sql.ErrNoRows}
 return scanDomain(rows)
}
func (d *DB) Domain(domain string)(DomainExpiry,error){
 rows,err:=d.Query(`SELECT `+domainColumns+` FROM domain_expiry d LEFT JOIN monitor_domains md ON md.domain=d.domain WHERE d.domain=? GROUP BY d.domain`,domain)
 if err!=nil{return DomainExpiry{},err}
 defer rows.Close()
 if !rows.Next(){return DomainExpiry{},sql.ErrNoRows}
 return scanDomain(rows)
}
func (d *DB) SaveDomainManual(domain,date string)error{
 if date!="" {
  parsed,err:=time.Parse("2006-01-02",date);if err!=nil||parsed.Format("2006-01-02")!=date{return errors.New("expected YYYY-MM-DD")}
 }
 res,err:=d.Exec(`UPDATE domain_expiry SET manual_date=? WHERE domain=?`,date,domain)
 if err!=nil{return err}
 n,err:=res.RowsAffected();if err!=nil{return err}
 if n==0{return sql.ErrNoRows}
 return nil
}
func (d *DB) DueDomains(now time.Time,limit int)([]DomainExpiry,error){
 if limit<1||limit>50{limit=10}
 rows,err:=d.Query(`SELECT `+domainColumns+` FROM domain_expiry d JOIN monitor_domains md ON md.domain=d.domain
 WHERE (d.next_check='' OR d.next_check<=? OR (d.rdap_date='' AND d.last_error LIKE 'RDAP is unavailable%' AND (d.domain LIKE '%.ru' OR d.domain LIKE '%.xn--p1ai'))) GROUP BY d.domain ORDER BY d.next_check,d.domain LIMIT ?`,now.UTC().Format(time.RFC3339),limit)
 if err!=nil{return nil,err}
 defer rows.Close()
 out:=[]DomainExpiry{}
 for rows.Next(){v,e:=scanDomain(rows);if e!=nil{return nil,e};out=append(out,v)}
 return out,rows.Err()
}
func (d *DB) RecordDomainLookup(ctx context.Context,domain string,expiry time.Time,source string,lookupErr error)error{
 now:=time.Now().UTC()
 next:=now.Add(24*time.Hour)
 errText:=""
 date:=""
 if lookupErr!=nil {
  errText=lookupErr.Error()
  if len(errText)>400{errText=errText[:400]}
  next=now.Add(24*time.Hour)
 }else{date=expiry.Format("2006-01-02")}
 // Preserve last successful expiry when RDAP returns a temporary error.
 if lookupErr==nil{
  _,err:=d.ExecContext(ctx,`UPDATE domain_expiry SET rdap_date=?,rdap_source=?,last_error='',last_checked=?,next_check=? WHERE domain=?`,date,source,now.Format(time.RFC3339),next.Format(time.RFC3339),domain)
  return err
 }
 _,err:=d.ExecContext(ctx,`UPDATE domain_expiry SET last_error=?,last_checked=?,next_check=? WHERE domain=?`,errText,now.Format(time.RFC3339),next.Format(time.RFC3339),domain)
 return err
}
func alertLevel(days int)string{
 if days<=0{return "EXPIRED"}
 if days<=1{return "1"}
 if days<=7{return "7"}
 if days<=14{return "14"}
 if days<=30{return "30"}
 return ""
}
func (d *DB) QueueDomainAlerts(ctx context.Context,now time.Time)error{
 list,err:=d.Domains();if err!=nil{return err}
 for _,v:=range list {
  if !v.HasDate{continue}
  v.Calculate(now)
  level:=alertLevel(v.DaysLeft)
  if level==""{continue}
  tx,e:=d.BeginTx(ctx,nil);if e!=nil{return e}
  var active int
  e=tx.QueryRowContext(ctx,`SELECT COUNT(*) FROM telegram_settings WHERE id=1 AND enabled=1 AND chat_id<>'' AND token_ciphertext<>''`).Scan(&active)
  if e!=nil{tx.Rollback();return e}
  if active==0 {tx.Rollback();continue}
  res,e:=tx.ExecContext(ctx,`INSERT OR IGNORE INTO domain_alerts(domain,expiry_date,level,queued_at) VALUES(?,?,?,?)`,v.Domain,v.EffectiveDate,level,now.UTC().Format(time.RFC3339))
  if e!=nil{tx.Rollback();return e}
  n,e:=res.RowsAffected();if e!=nil{tx.Rollback();return e}
  if n>0 {
   status:=fmt.Sprintf("⚠️ До окончания регистрации домена %s осталось %d дн.",v.Domain,v.DaysLeft)
   if level=="EXPIRED"{status=fmt.Sprintf("🔴 Срок регистрации домена %s наступил или прошёл.",v.Domain)}
   message:=status+"\nДата: "+v.EffectiveDate+"\nИсточник: "+v.Source
   _,e=tx.ExecContext(ctx,`INSERT INTO telegram_outbox(message,next_attempt_at) VALUES(?,?)`,message,now.UTC().Format(time.RFC3339))
   if e!=nil{tx.Rollback();return e}
  }
  if e=tx.Commit();e!=nil{return e}
 }
 return nil
}
func (d *DB) DomainWarnings()([]DomainExpiry,error){
 list,err:=d.Domains();if err!=nil{return nil,err}
 out:=[]DomainExpiry{}
 for _,v:=range list{if v.HasDate && v.DaysLeft<=30{out=append(out,v)}}
 return out,nil
}
var _ = strings.TrimSpace
