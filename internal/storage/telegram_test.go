package storage

import (
 "context"
 "path/filepath"
 "testing"
 "time"
)

func TestTelegramOutboxLifecycle(t *testing.T) {
 db,err:=Open(filepath.Join(t.TempDir(),"test.db"));if err!=nil{t.Fatal(err)}
 defer db.Close()
 err=db.SaveTelegram(TelegramSettings{Enabled:true,NotifyDown:true,NotifyUp:true,ChatID:"123",EncryptedToken:"cipher"})
 if err!=nil{t.Fatal(err)}
 id,err:=db.CreateMonitor("Example","https://example.com",15,5,true);if err!=nil{t.Fatal(err)}
 ctx:=context.Background()
 for i:=0;i<3;i++ {
  err=db.Record(ctx,id,Check{CheckedAt:time.Now().UTC().Add(time.Duration(i)*time.Second).Format(time.RFC3339),Success:false,Error:"timeout"})
  if err!=nil{t.Fatal(err)}
 }
 first,err:=db.NextTelegramMessage(ctx);if err!=nil{t.Fatal(err)}
 if first.ID==0 {t.Fatal("expected one DOWN message")}
 // Additional failures must not create more messages.
 err=db.Record(ctx,id,Check{CheckedAt:time.Now().UTC().Add(4*time.Second).Format(time.RFC3339),Success:false,Error:"timeout"})
 if err!=nil{t.Fatal(err)}
 if err=db.MarkTelegramSent(ctx,first.ID);err!=nil{t.Fatal(err)}
 if _,err=db.NextTelegramMessage(ctx);err==nil{t.Fatal("duplicate DOWN message")}
 for i:=0;i<2;i++{
  err=db.Record(ctx,id,Check{CheckedAt:time.Now().UTC().Add(time.Duration(i+5)*time.Second).Format(time.RFC3339),Success:true,Code:200})
  if err!=nil{t.Fatal(err)}
 }
 recovered,err:=db.NextTelegramMessage(ctx);if err!=nil{t.Fatal(err)}
 if recovered.ID==first.ID {t.Fatal("expected new recovery notification")}
}
