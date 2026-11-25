package storage

import (
 "context"
 "path/filepath"
 "testing"
)
func TestSitemapBaselineAndChanges(t *testing.T){
 db,err:=Open(filepath.Join(t.TempDir(),"sitemap.db"));if err!=nil{t.Fatal(err)}
 defer db.Close()
 id,err:=db.CreateMonitor("Example","https://example.com",60,10,true);if err!=nil{t.Fatal(err)}
 if err=db.ConfigureSitemap(id,true,"https://example.com/sitemap.xml");err!=nil{t.Fatal(err)}
 ctx:=context.Background()
 if err=db.RecordSitemap(ctx,id,[]string{"https://example.com/a","https://example.com/b"},1);err!=nil{t.Fatal(err)}
 changes,err:=db.SitemapChanges(id,100)
 if err!=nil||len(changes)!=0{t.Fatalf("baseline should not generate events: %+v %v",changes,err)}
 if err=db.RecordSitemap(ctx,id,[]string{"https://example.com/b","https://example.com/c"},2);err!=nil{t.Fatal(err)}
 state,err:=db.SitemapForMonitor(id)
 if err!=nil||state.Total!=2||state.Added!=1||state.Removed!=1||state.Files!=2{t.Fatalf("unexpected state %+v: %v",state,err)}
 changes,err=db.SitemapChanges(id,100)
 if err!=nil||len(changes)!=2{t.Fatalf("expected two change events: %+v %v",changes,err)}
 if err=db.RecordSitemapError(ctx,id,context.DeadlineExceeded);err!=nil{t.Fatal(err)}
 after,err:=db.SitemapForMonitor(id)
 if err!=nil||after.Total!=2||after.Added!=1||after.Removed!=1||after.Status!="ERROR"{t.Fatalf("failure must keep last successful snapshot: %+v %v",after,err)}
}


func TestSitemapSourceSwitchCreatesNewBaseline(t *testing.T){
 db,err:=Open(filepath.Join(t.TempDir(),"switch.db"));if err!=nil{t.Fatal(err)}
 defer db.Close()
 id,err:=db.CreateMonitor("Example","https://example.com",60,10,true);if err!=nil{t.Fatal(err)}
 ctx:=context.Background()
 if err=db.ConfigureSitemap(id,true,"https://example.com/post-sitemap.xml");err!=nil{t.Fatal(err)}
 post:=[]string{"https://example.com/a","https://example.com/b"}
 if err=db.RecordSitemap(ctx,id,post,1);err!=nil{t.Fatal(err)}
 if err=db.ConfigureSitemap(id,true,"https://example.com/sitemap_index.xml");err!=nil{t.Fatal(err)}
 // A failed scan must preserve the old baseline until a successful replacement.
 if err=db.RecordSitemapError(ctx,id,context.DeadlineExceeded);err!=nil{t.Fatal(err)}
 if err=db.RecordSitemap(ctx,id,[]string{"https://example.com/a","https://example.com/b","https://example.com/c"},4);err!=nil{t.Fatal(err)}
 s,err:=db.SitemapForMonitor(id)
 if err!=nil||s.Total!=3||s.Added!=0||s.Removed!=0{t.Fatalf("switch must rebaseline, got %+v %v",s,err)}
 changes,err:=db.SitemapChanges(id,100)
 if err!=nil||len(changes)!=0{t.Fatalf("switch created false events: %+v %v",changes,err)}
 if err=db.RecordSitemap(ctx,id,[]string{"https://example.com/a","https://example.com/c","https://example.com/d"},4);err!=nil{t.Fatal(err)}
 s,err=db.SitemapForMonitor(id)
 if err!=nil||s.Added!=1||s.Removed!=1{t.Fatalf("second scan should diff new source: %+v %v",s,err)}
 changes,err=db.SitemapChanges(id,100)
 if err!=nil||len(changes)!=2{t.Fatalf("expected actual changes: %+v %v",changes,err)}
}
