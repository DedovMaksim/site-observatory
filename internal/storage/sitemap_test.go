package storage

import (
 "context"
 "fmt"
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


func TestSitemapTelegramTransitionsAndBulkRemoval(t *testing.T) {
 db,err:=Open(filepath.Join(t.TempDir(),"alerts.db"));if err!=nil{t.Fatal(err)}
 defer db.Close()
 id,err:=db.CreateMonitor("Demo","https://example.com",60,10,true);if err!=nil{t.Fatal(err)}
 if err=db.ConfigureSitemap(id,true,"https://example.com/sitemap.xml");err!=nil{t.Fatal(err)}
 ctx:=context.Background()
 _,err=db.Exec(`UPDATE telegram_settings SET enabled=1,chat_id='123',token_ciphertext='cipher' WHERE id=1`);if err!=nil{t.Fatal(err)}
 countMessages:=func()int{var n int;if e:=db.QueryRow(`SELECT COUNT(*) FROM telegram_outbox`).Scan(&n);e!=nil{t.Fatal(e)};return n}
 if err=db.RecordSitemapError(ctx,id,context.DeadlineExceeded);err!=nil{t.Fatal(err)}
 if err=db.RecordSitemapError(ctx,id,context.DeadlineExceeded);err!=nil{t.Fatal(err)}
 if n:=countMessages();n!=1{t.Fatalf("expected one error alert, got %d",n)}
 list:=make([]string,20)
 for i:=range list{list[i]=fmt.Sprintf("https://example.com/p/%d",i)}
 if err=db.RecordSitemap(ctx,id,list,1);err!=nil{t.Fatal(err)}
 if n:=countMessages();n!=2{t.Fatalf("expected one recovery alert, got %d",n)}
 // First snapshot after a failure is a baseline: no false removal alert.
 if err=db.RecordSitemap(ctx,id,list[:9],1);err!=nil{t.Fatal(err)}
 if n:=countMessages();n!=3{t.Fatalf("expected one bulk removal warning, got %d",n)}
 if err=db.RecordSitemap(ctx,id,list[:9],1);err!=nil{t.Fatal(err)}
 if n:=countMessages();n!=3{t.Fatalf("repeated identical snapshot must not alert, got %d",n)}
 snapshots,err:=db.SitemapSnapshots(id,10)
 if err!=nil||len(snapshots)!=3{t.Fatalf("snapshots %+v %v",snapshots,err)}
}
func TestSitemapAlertRequiresConfiguredTelegram(t *testing.T){
 db,err:=Open(filepath.Join(t.TempDir(),"disabled.db"));if err!=nil{t.Fatal(err)}
 defer db.Close()
 id,err:=db.CreateMonitor("Demo","https://example.com",60,10,true);if err!=nil{t.Fatal(err)}
 if err=db.ConfigureSitemap(id,true,"https://example.com/sitemap.xml");err!=nil{t.Fatal(err)}
 if err=db.RecordSitemapError(context.Background(),id,context.DeadlineExceeded);err!=nil{t.Fatal(err)}
 var count int
 if err=db.QueryRow(`SELECT COUNT(*) FROM telegram_outbox`).Scan(&count);err!=nil{t.Fatal(err)}
 if count!=0{t.Fatalf("disabled Telegram must not queue alerts: %d",count)}
}
