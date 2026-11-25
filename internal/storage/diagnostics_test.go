package storage

import (
 "context"
 "path/filepath"
 "testing"
 "time"
)

func TestDiagnosticSnapshotsAndUptime(t *testing.T) {
 db,err:=Open(filepath.Join(t.TempDir(),"test.db"));if err!=nil{t.Fatal(err)}
 defer db.Close()
 id,err:=db.CreateMonitor("Example","https://example.com",60,10,true);if err!=nil{t.Fatal(err)}
 ctx:=context.Background()
 first:=Diagnostic{MonitorID:id,CheckedAt:time.Now().UTC().Format(time.RFC3339),TLSStatus:"OK",TLSDays:120,DNSSummary:"1.1.1.1",Title:"Example",SEOStatus:"OK"}
 if err=db.SaveDiagnostic(ctx,first);err!=nil{t.Fatal(err)}
 got,err:=db.Diagnostic(id);if err!=nil{t.Fatal(err)}
 if got.DNSChanged {t.Fatal("first snapshot cannot be a change")}
 first.DNSSummary="8.8.8.8"
 if err=db.SaveDiagnostic(ctx,first);err!=nil{t.Fatal(err)}
 got,err=db.Diagnostic(id);if err!=nil{t.Fatal(err)}
 if !got.DNSChanged {t.Fatal("DNS change not detected")}
 for i:=0;i<3;i++ {
  err=db.Record(ctx,id,Check{CheckedAt:time.Now().UTC().Format(time.RFC3339),Success:i<2,Code:200,Latency:100})
  if err!=nil{t.Fatal(err)}
 }
 stats,err:=db.Uptime(id,time.Now().Add(-24*time.Hour))
 if err!=nil{t.Fatal(err)}
 if stats.Total!=3 || stats.Successful!=2 || stats.AvgLatency!=100 {t.Fatalf("unexpected uptime stats %+v",stats)}
 points,err:=db.LastLatencies(id,100);if err!=nil||len(points)!=3{t.Fatalf("unexpected latency points %v %v",points,err)}
}

func TestUptimeFractionalAverageDoesNotBreakDetail(t *testing.T){
 db,err:=Open(filepath.Join(t.TempDir(),"fractional.db"))
 if err!=nil{t.Fatal(err)}
 defer db.Close()
 id,err:=db.CreateMonitor("Fractional","https://example.com",60,10,true)
 if err!=nil{t.Fatal(err)}
 now:=time.Now().UTC().Format(time.RFC3339)
 for _,latency:=range []int64{725,726}{
  if err=db.Record(context.Background(),id,Check{CheckedAt:now,Code:200,Latency:latency,Success:true});err!=nil{t.Fatal(err)}
 }
 stats,err:=db.Uptime(id,time.Now().Add(-time.Hour))
 if err!=nil{t.Fatalf("fractional SQL AVG must not fail integer scan: %v",err)}
 if stats.Total!=2||stats.AvgLatency!=725{t.Fatalf("unexpected truncated latency: %+v",stats)}
}
