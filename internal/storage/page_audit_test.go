package storage

import (
 "context"
 "path/filepath"
 "testing"
 "time"
)
func TestPageAuditOptInAndQueue(t *testing.T){
 db,e:=Open(filepath.Join(t.TempDir(),"page.db"));if e!=nil{t.Fatal(e)}
 defer db.Close()
 id,e:=db.CreateMonitor("Example","https://example.com",60,10,true);if e!=nil{t.Fatal(e)}
 if e=db.ConfigureSitemap(id,true,"https://example.com/sitemap.xml");e!=nil{t.Fatal(e)}
 if e=db.RecordSitemap(context.Background(),id,[]string{"https://example.com/a","https://example.com/b"},1);e!=nil{t.Fatal(e)}
 if q,e:=db.PageAuditDue(time.Now(),5);e!=nil||len(q)!=0{t.Fatalf("unexpected due while disabled %+v %v",q,e)}
 if e=db.ConfigurePageAudit(id,true);e!=nil{t.Fatal(e)}
 q,e:=db.PageAuditDue(time.Now(),5)
 if e!=nil||len(q)!=2{t.Fatalf("expected 2 pages %+v %v",q,e)}
 if e=db.SavePageAudit(context.Background(),id,PageAuditResult{URL:q[0].URL,Status:200,Title:"Old",Canonical:"https://example.com/a"});e!=nil{t.Fatal(e)}
 events,e:=db.PageAuditEvents(id)
 if e!=nil||len(events)!=0{t.Fatalf("baseline must not generate events %+v %v",events,e)}
 q,e=db.PageAuditDue(time.Now(),5)
 if e!=nil||len(q)!=1{t.Fatalf("should delay checked page by 24h %+v %v",q,e)}
 if e=db.SavePageAudit(context.Background(),id,PageAuditResult{URL:"https://example.com/a",Status:404,Title:"Changed",Robots:"noindex",Noindex:true});e!=nil{t.Fatal(e)}
 results,e:=db.PageAuditResults(id,10)
 if e!=nil||len(results)!=1||results[0].Status!=404||!results[0].Noindex{t.Fatalf("results %+v %v",results,e)}
 events,e=db.PageAuditEvents(id)
 if e!=nil||len(events)==0{t.Fatalf("changes should be recorded %+v %v",events,e)}
}
