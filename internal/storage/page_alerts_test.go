package storage

import (
 "context"
 "errors"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func newPageAlertTestDB(t *testing.T)(*DB,int64,string){
 t.Helper()
 db,err:=Open(filepath.Join(t.TempDir(),"alerts.db"));if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){db.Close()})
 id,err:=db.CreateMonitor("Example","https://example.com",60,10,true);if err!=nil{t.Fatal(err)}
 url:="https://example.com/page/"
 if err=db.ConfigureSitemap(id,true,"https://example.com/sitemap.xml");err!=nil{t.Fatal(err)}
 if err=db.RecordSitemap(context.Background(),id,[]string{url},1);err!=nil{t.Fatal(err)}
 if err=db.ConfigurePageAudit(id,true);err!=nil{t.Fatal(err)}
 _,err=db.Exec(`UPDATE telegram_settings SET enabled=1,chat_id='123',token_ciphertext='encrypted' WHERE id=1`)
 if err!=nil{t.Fatal(err)}
 return db,id,url
}
func savePageForAlert(t *testing.T,db *DB,id int64,url string,status int,title string,noindex bool){
 t.Helper()
 v:=PageAuditResult{URL:url,Status:status,IsHTML:status==200,Title:title,Noindex:noindex,Canonical:url}
 if status!=200{v.Title="";v.Canonical="";v.IsHTML=false}
 if err:=db.SavePageAudit(context.Background(),id,v);err!=nil{t.Fatal(err)}
}
func alertMessageCount(t *testing.T,db *DB)int{
 t.Helper()
 var n int
 if err:=db.QueryRow(`SELECT COUNT(*) FROM telegram_outbox`).Scan(&n);err!=nil{t.Fatal(err)}
 return n
}
func forcePageConfirmationDue(t *testing.T,db *DB,id int64){
 t.Helper()
 _,err:=db.Exec(`UPDATE page_audit_alerts SET confirm_at=? WHERE monitor_id=? AND state='PENDING'`,time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),id)
 if err!=nil{t.Fatal(err)}
}
func TestPageAlertsConfirmDeduplicateAndRecover(t *testing.T){
 db,id,url:=newPageAlertTestDB(t)
 savePageForAlert(t,db,id,url,200,"Healthy",false)
 if n:=alertMessageCount(t,db);n!=0{t.Fatalf("baseline alerted: %d",n)}
 savePageForAlert(t,db,id,url,404,"",false)
 pending,active,err:=db.PageAuditAlertSummary(id)
 if err!=nil||pending!=1||active!=0{t.Fatalf("after first 404: %d,%d,%v",pending,active,err)}
 if n:=alertMessageCount(t,db);n!=0{t.Fatalf("unconfirmed issue alerted: %d",n)}
 if due,err:=db.PageAuditDue(time.Now(),5);err!=nil||len(due)!=0{t.Fatalf("premature confirmation: %+v %v",due,err)}
 forcePageConfirmationDue(t,db,id)
 if due,err:=db.PageAuditDue(time.Now(),5);err!=nil||len(due)!=1{t.Fatalf("confirmation not scheduled: %+v %v",due,err)}
 savePageForAlert(t,db,id,url,404,"",false)
 if n:=alertMessageCount(t,db);n!=1{t.Fatalf("confirmed 404 should alert once, got %d",n)}
 savePageForAlert(t,db,id,url,404,"",false)
 if n:=alertMessageCount(t,db);n!=1{t.Fatalf("duplicate 404 alert: %d",n)}
 savePageForAlert(t,db,id,url,200,"Back online",false)
 if n:=alertMessageCount(t,db);n!=2{t.Fatalf("recovery should alert once: %d",n)}
 pending,active,err=db.PageAuditAlertSummary(id)
 if err!=nil||pending!=0||active!=0{t.Fatalf("expected cleared alerts: %d %d %v",pending,active,err)}
 savePageForAlert(t,db,id,url,200,"Back online",false)
 if n:=alertMessageCount(t,db);n!=2{t.Fatalf("duplicate recovery: %d",n)}
 var message string
 if err=db.QueryRow(`SELECT message FROM telegram_outbox ORDER BY id LIMIT 1`).Scan(&message);err!=nil{t.Fatal(err)}
 if !strings.Contains(message,url)||!strings.Contains(message,"404"){t.Fatalf("missing page/status details: %s",message)}
}
func TestPageAlertTransientFailureAndSilentBaseline(t *testing.T){
 db,id,url:=newPageAlertTestDB(t)
 savePageForAlert(t,db,id,url,200,"Healthy",false)
 savePageForAlert(t,db,id,url,500,"",false)
 savePageForAlert(t,db,id,url,200,"Healthy",false)
 if n:=alertMessageCount(t,db);n!=0{t.Fatalf("transient failure alerted: %d",n)}
 if p,_,err:=db.PageAuditAlertSummary(id);err!=nil||p!=0{t.Fatalf("transient pending wasn't cleared: %d %v",p,err)}
 // Baseline page that starts bad must not generate a warning or recovery.
 another:="https://example.com/new/"
 if err:=db.RecordSitemap(context.Background(),id,[]string{url,another},1);err!=nil{t.Fatal(err)}
 savePageForAlert(t,db,id,another,404,"",false)
 savePageForAlert(t,db,id,another,200,"Healthy",false)
 if n:=alertMessageCount(t,db);n!=0{t.Fatalf("preexisting baseline failure produced alert: %d",n)}
}
func TestPageAlertsNoindexTitleAndTransportFailures(t *testing.T){
 db,id,url:=newPageAlertTestDB(t)
 savePageForAlert(t,db,id,url,200,"Healthy",false)
 savePageForAlert(t,db,id,url,200,"",true)
 if p,_,err:=db.PageAuditAlertSummary(id);err!=nil||p!=2{t.Fatalf("expected title+noindex confirmations: %d %v",p,err)}
 // A temporary failed HTTP request must not clear the SEO alert states.
 savePageForAlert(t,db,id,url,503,"",false)
 if p,_,err:=db.PageAuditAlertSummary(id);err!=nil||p!=3{t.Fatalf("HTTP+SEO pending lost on transient error: %d %v",p,err)}
 forcePageConfirmationDue(t,db,id)
 savePageForAlert(t,db,id,url,200,"",true)
 if n:=alertMessageCount(t,db);n!=2{t.Fatalf("expected two SEO warnings, got %d",n)}
 savePageForAlert(t,db,id,url,200,"Recovered title",false)
 if n:=alertMessageCount(t,db);n!=4{t.Fatalf("expected two recoveries, got %d",n)}
}
func TestPageAlertUnconfiguredTelegramDoesNotQueueRecovery(t *testing.T){
 db,id,url:=newPageAlertTestDB(t)
 savePageForAlert(t,db,id,url,200,"Healthy",false)
 _,err:=db.Exec(`UPDATE telegram_settings SET enabled=0 WHERE id=1`);if err!=nil{t.Fatal(err)}
 savePageForAlert(t,db,id,url,404,"",false)
 forcePageConfirmationDue(t,db,id)
 savePageForAlert(t,db,id,url,404,"",false)
 _,err=db.Exec(`UPDATE telegram_settings SET enabled=1 WHERE id=1`);if err!=nil{t.Fatal(err)}
 savePageForAlert(t,db,id,url,200,"Healthy",false)
 if n:=alertMessageCount(t,db);n!=0{t.Fatalf("recovery with no original alert: %d",n)}
}
func TestPageAlertDBErrorIsPropagated(t *testing.T){
 db,id,url:=newPageAlertTestDB(t)
 if err:=db.SavePageAudit(context.Background(),id,PageAuditResult{URL:url,Status:200,Title:"OK",IsHTML:true});err!=nil{t.Fatal(err)}
 if err:=db.Close();err!=nil && !errors.Is(err,context.Canceled){t.Fatal(err)}
 if err:=db.SavePageAudit(context.Background(),id,PageAuditResult{URL:url,Status:500});err==nil{t.Fatal("expected closed database error")}
}
