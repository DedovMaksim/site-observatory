package storage

import (
 "context"
 "path/filepath"
 "testing"
 "errors"
)
func TestRobotsBaselinesAlertsRecovery(t *testing.T){
 db,err:=Open(filepath.Join(t.TempDir(),"robots.db"));if err!=nil{t.Fatal(err)}
 defer db.Close()
 id,err:=db.CreateMonitor("Example","https://example.com",60,10,true);if err!=nil{t.Fatal(err)}
 if err=db.ConfigureRobots(id,true);err!=nil{t.Fatal(err)}
 ctx:=context.Background()
 _,err=db.Exec(`UPDATE telegram_settings SET enabled=1,chat_id='123',token_ciphertext='cipher' WHERE id=1`);if err!=nil{t.Fatal(err)}
 messages:=func()int{var n int;if e:=db.QueryRow(`SELECT COUNT(*) FROM telegram_outbox`).Scan(&n);e!=nil{t.Fatal(e)};return n}
 if err=db.RecordRobots(ctx,id,"User-agent: *\nDisallow: /private\n");err!=nil{t.Fatal(err)}
 if n:=messages();n!=0{t.Fatalf("baseline should not alert: %d",n)}
 if err=db.RecordRobots(ctx,id,"User-agent: *\nDisallow: /\nSitemap: https://example.com/sitemap.xml\n");err!=nil{t.Fatal(err)}
 if n:=messages();n!=2{t.Fatalf("root block plus sitemap change should alert twice, got %d",n)}
 if err=db.RecordRobots(ctx,id,"User-agent: *\nDisallow: /\nSitemap: https://example.com/sitemap.xml\n");err!=nil{t.Fatal(err)}
 if n:=messages();n!=2{t.Fatalf("repeat should not alert: %d",n)}
 if err=db.RecordRobotsError(ctx,id,errors.New("HTTP 503"));err!=nil{t.Fatal(err)}
 if err=db.RecordRobotsError(ctx,id,errors.New("HTTP 503"));err!=nil{t.Fatal(err)}
 if n:=messages();n!=3{t.Fatalf("error should alert once: %d",n)}
 if err=db.RecordRobots(ctx,id,"User-agent: *\nDisallow: /\nSitemap: https://example.com/sitemap.xml\n");err!=nil{t.Fatal(err)}
 if n:=messages();n!=4{t.Fatalf("recovery should alert once: %d",n)}
 history,err:=db.RobotsChanges(id)
 if err!=nil||len(history)<4{t.Fatalf("history %+v %v",history,err)}
}
func TestRobotsOptIn(t *testing.T){
 db,err:=Open(filepath.Join(t.TempDir(),"disabled.db"));if err!=nil{t.Fatal(err)}
 defer db.Close()
 id,err:=db.CreateMonitor("Example","https://example.com",60,10,true);if err!=nil{t.Fatal(err)}
 if err=db.RecordRobots(context.Background(),id,"User-agent: *\nDisallow: /");err!=nil{t.Fatal(err)}
 state,err:=db.RobotsForMonitor(id)
 if err!=nil||state.Enabled{t.Fatalf("unexpected state %+v %v",state,err)}
}
