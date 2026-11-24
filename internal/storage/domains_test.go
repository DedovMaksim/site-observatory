package storage

import (
 "context"
 "path/filepath"
 "testing"
 "time"
)
func TestDomainMigrationAndDeduplicatedAlerts(t *testing.T){
 db,err:=Open(filepath.Join(t.TempDir(),"test.db"));if err!=nil{t.Fatal(err)}
 defer db.Close()
 a,err:=db.CreateMonitor("A","https://www.example.com",60,10,true);if err!=nil{t.Fatal(err)}
 b,err:=db.CreateMonitor("B","https://shop.example.com",60,10,true);if err!=nil{t.Fatal(err)}
 if err=db.SyncMonitorDomain(a,"https://www.example.com");err!=nil{t.Fatal(err)}
 if err=db.SyncMonitorDomain(b,"https://shop.example.com");err!=nil{t.Fatal(err)}
 list,err:=db.Domains()
 if err!=nil||len(list)!=1||list[0].MonitorCount!=2{t.Fatalf("domains %+v %v",list,err)}
 expiry:=time.Now().UTC().AddDate(0,0,14).Format("2006-01-02")
 if err=db.SaveDomainManual("example.com",expiry);err!=nil{t.Fatal(err)}
 if err=db.RecordDomainLookup(context.Background(),"example.com",time.Now().UTC().AddDate(1,0,0),"RDAP (expiration)",nil);err!=nil{t.Fatal(err)}
 domain,err:=db.Domain("example.com")
 if err!=nil||domain.Source!="Вручную"||domain.EffectiveDate!=expiry{t.Fatalf("manual date overwritten %+v %v",domain,err)}
 _,err=db.Exec(`UPDATE telegram_settings SET enabled=1,chat_id='123',token_ciphertext='ciphertext' WHERE id=1`)
 if err!=nil{t.Fatal(err)}
 now:=time.Now()
 for i:=0;i<2;i++{if err=db.QueueDomainAlerts(context.Background(),now);err!=nil{t.Fatal(err)}}
 var count int
 if err=db.QueryRow(`SELECT COUNT(*) FROM telegram_outbox`).Scan(&count);err!=nil||count!=1{t.Fatalf("alerts count %d %v",count,err)}
}
