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


func TestPageAuditSearchFiltersAndPagination(t *testing.T){
 db,err:=Open(filepath.Join(t.TempDir(),"filter.db"));if err!=nil{t.Fatal(err)}
 defer db.Close()
 id,err:=db.CreateMonitor("Example","https://example.com",60,10,true);if err!=nil{t.Fatal(err)}
 ctx:=context.Background()
 urls:=[]string{"https://example.com/a","https://example.com/b","https://example.com/c","https://example.com/d","https://example.com/percent%25"}
 if err=db.ConfigureSitemap(id,true,"https://example.com/sitemap.xml");err!=nil{t.Fatal(err)}
 if err=db.RecordSitemap(ctx,id,urls,1);err!=nil{t.Fatal(err)}
 cases:=[]PageAuditResult{
  {URL:urls[0],Status:200,Title:"Специи и травы",Canonical:urls[0]},
  {URL:urls[1],Status:404,Title:"Не найдено",Canonical:urls[1]},
  {URL:urls[2],Status:200,Canonical:"",Noindex:true,Robots:"noindex"},
  {URL:urls[3],Status:200,Title:"Другая",Canonical:urls[0]},
  {URL:urls[4],Status:200,Title:"Percent 100%",Canonical:urls[4]},
 }
 for _,v:=range cases{if err=db.SavePageAudit(ctx,id,v);err!=nil{t.Fatal(err)}}
 checks:=[]struct{filter,search string;want int}{
  {"all","",5},
  {"http","",1},
  {"noindex","",1},
  {"title","",1},
  {"canonical","",1},
  {"mismatch","",1},
  {"all","Специи",1},
  {"all","percent%25",1},
  {"all","percent_",0},
  {"http","Специи",0},
 }
 for _,tc:=range checks{
  page,e:=db.PageAuditFiltered(PageAuditFilter{MonitorID:id,Status:tc.filter,Search:tc.search,Page:1,PageSize:25})
  if e!=nil||page.Total!=tc.want{t.Errorf("filter=%s search=%s got %d,err=%v want %d",tc.filter,tc.search,page.Total,e,tc.want)}
 }
 p,e:=db.PageAuditFiltered(PageAuditFilter{MonitorID:id,Status:"all",Page:1,PageSize:2})
 if e!=nil||len(p.Results)!=2||!p.HasNext||p.HasPrevious{t.Fatalf("page 1 %+v %v",p,e)}
 p,e=db.PageAuditFiltered(PageAuditFilter{MonitorID:id,Status:"all",Page:3,PageSize:2})
 if e!=nil||len(p.Results)!=1||p.HasNext||!p.HasPrevious{t.Fatalf("page 3 %+v %v",p,e)}
 stats,e:=db.PageAuditIssueCounts(id)
 if e!=nil||stats.HTTP!=1||stats.Noindex!=1||stats.Title!=1||stats.Canonical!=1||stats.CanonicalMismatch!=1{t.Fatalf("issue counts %+v %v",stats,e)}
 if _,e=db.PageAuditFiltered(PageAuditFilter{MonitorID:id,Status:"unexpected"});e==nil{t.Fatal("unknown filter accepted")}
}
