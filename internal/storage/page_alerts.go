package storage

import (
 "context"
 "database/sql"
 "errors"
 "fmt"
 "strings"
 "time"
)

// The two-minute confirmation window is applied to each newly detected
// problem. A confirmation is scheduled through the normal rate-limited
// page-audit queue, rather than an additional unbounded HTTP worker.
const pageAlertConfirmationDelay = 2 * time.Minute

type pageAlertKind struct {
 Name string
 Issue bool
 Severity string
 Description string
}
func pageAlertKinds(v PageAuditResult) []pageAlertKind {
 // Avoid reporting missing metadata from HTTP failures and non-HTML bodies.
 // Metadata alerts are meaningful only for successfully fetched HTML.
 htmlOK:=v.Status>=200 && v.Status<300 && v.Error=="" && v.IsHTML
 return []pageAlertKind{
  {Name:"HTTP",Issue:v.Status==0||v.Status>=400||v.Error!="",Severity:"🔴 Критическая",Description:fmt.Sprintf("HTTP: %d",v.Status)},
  {Name:"NOINDEX",Issue:htmlOK&&v.Noindex,Severity:"🔴 Критическая",Description:"Обнаружена директива noindex"},
  {Name:"TITLE",Issue:htmlOK&&strings.TrimSpace(v.Title)=="",Severity:"🟠 Высокая",Description:"Отсутствует Title"},
 }
}
func alertIssueForKind(list []pageAlertKind, kind string) bool {
 for _,item:=range list {if item.Name==kind{return item.Issue}}
 return false
}
func (d *DB) PageAuditAlertSummary(id int64)(pending,active int,err error) {
 err=d.QueryRow(`SELECT
 COALESCE(SUM(CASE WHEN state='PENDING' THEN 1 ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN state='ACTIVE' AND notified=1 THEN 1 ELSE 0 END),0)
 FROM page_audit_alerts WHERE monitor_id=?`,id).Scan(&pending,&active)
 return
}
// updatePageAlerts is always executed inside the same transaction as the
// page snapshot. This makes alerts and recovery messages idempotent.
func updatePageAlerts(ctx context.Context, tx *sql.Tx, monitorID int64, old, current PageAuditResult, baseline bool, now time.Time) error {
 var name string
 var enabled int
 err:=tx.QueryRowContext(ctx,`SELECT m.name,a.enabled FROM monitors m JOIN page_audit_settings a ON a.monitor_id=m.id WHERE m.id=?`,monitorID).Scan(&name,&enabled)
 if errors.Is(err,sql.ErrNoRows) || enabled!=1 {return nil}
 if err!=nil{return err}
 previousIssues:=pageAlertKinds(old)
 currentIssues:=pageAlertKinds(current)
 for _,kind:=range currentIssues {
  var state,confirmAt string
  var notified int
  err=tx.QueryRowContext(ctx,`SELECT state,confirm_at,notified FROM page_audit_alerts WHERE monitor_id=? AND url=? AND kind=?`,monitorID,current.URL,kind.Name).Scan(&state,&confirmAt,&notified)
  missing:=errors.Is(err,sql.ErrNoRows)
  if err!=nil&&!missing{return err}
  // For the initial baseline, or when upgrading an existing already-failing
  // page, preserve the issue silently instead of inventing a new incident.
  previouslyBad:=!baseline&&alertIssueForKind(previousIssues,kind.Name)
  if missing {
   if !kind.Issue {continue}
   if baseline||previouslyBad {
    _,err=tx.ExecContext(ctx,`INSERT INTO page_audit_alerts(monitor_id,url,kind,state,notified) VALUES(?,?,?,'ACTIVE',0)`,monitorID,current.URL,kind.Name)
   }else{
    _,err=tx.ExecContext(ctx,`INSERT INTO page_audit_alerts(monitor_id,url,kind,state,confirm_at) VALUES(?,?,?,'PENDING',?)`,monitorID,current.URL,kind.Name,now.Add(pageAlertConfirmationDelay).Format(time.RFC3339))
   }
   if err!=nil{return err}
   continue
  }
  if !kind.Issue {
   if state=="ACTIVE" && notified==1 {
    message:=fmt.Sprintf("🟢 Site Observatory: SEO-проблема устранена\nСайт: %s\nСтраница: %s\nПроверка: %s",name,current.URL,kind.Name)
    if err=enqueueSitemapAlert(ctx,tx,message);err!=nil{return err}
   }
   if _,err=tx.ExecContext(ctx,`DELETE FROM page_audit_alerts WHERE monitor_id=? AND url=? AND kind=?`,monitorID,current.URL,kind.Name);err!=nil{return err}
   continue
  }
  if state=="PENDING" {
   deadline,err:=time.Parse(time.RFC3339,confirmAt)
   if err!=nil{return fmt.Errorf("invalid SEO confirmation deadline: %w",err)}
   if !now.Before(deadline) {
    msg:=fmt.Sprintf("%s SEO-проблема подтверждена\nСайт: %s\nURL: %s\nПроблема: %s\nПодтверждена повторной проверкой.",kind.Severity,name,current.URL,kind.Description)
    // Do not send stale alerts if notifications were disabled at the time
    // of confirmation; preserve only the visible alert state.
    if err=enqueueSitemapAlert(ctx,tx,msg);err!=nil{return err}
    _,err=tx.ExecContext(ctx,`UPDATE page_audit_alerts SET state='ACTIVE',confirm_at='',notified=1 WHERE monitor_id=? AND url=? AND kind=?`,monitorID,current.URL,kind.Name)
    if err!=nil{return err}
   }
  }
 }
 return nil
}
