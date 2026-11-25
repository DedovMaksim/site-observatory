package notify

import (
 "path/filepath"
 "context"
 "net/http"
 "net/http/httptest"
 "encoding/json"
 "strings"
 "testing"
 "example.com/site-observatory/internal/storage"
)
func TestTokenEncryption(t *testing.T) {
 dir:=t.TempDir()
 db,err:=storage.Open(filepath.Join(dir,"test.db"));if err!=nil{t.Fatal(err)}
 defer db.Close()
 sender,err:=New(db,dir);if err!=nil{t.Fatal(err)}
 cipherText,err:=sender.Encrypt("123456:TEST-secret-token");if err!=nil{t.Fatal(err)}
 if cipherText=="123456:TEST-secret-token"{t.Fatal("plaintext token was stored")}
 got,err:=sender.decrypt(cipherText);if err!=nil{t.Fatal(err)}
 if got!="123456:TEST-secret-token"{t.Fatal("token roundtrip failed")}
 // Encryption must persist across restarts using the same private key file.
 restarted,err:=New(db,dir);if err!=nil{t.Fatal(err)}
 got,err=restarted.decrypt(cipherText);if err!=nil||got!="123456:TEST-secret-token"{t.Fatal("restart token decryption failed",err)}
}

func TestRelayDelivery(t *testing.T) {
 const secret="test-secret-not-for-production"
 calls:=0
 relay:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  calls++
  if r.Method!="POST"{t.Error("incorrect method",r.Method)}
  if got:=r.Header.Get("Authorization");got!="Bearer "+secret{t.Error("incorrect auth")}
  if r.Header.Get("Content-Type")!="application/json"{t.Error("incorrect content type")}
  var body struct{Text string `json:"text"`}
  if err:=json.NewDecoder(r.Body).Decode(&body);err!=nil || body.Text!="monitor down"{t.Error("unexpected body",body,err)}
  w.WriteHeader(http.StatusOK)
 }))
 defer relay.Close()
 t.Setenv("RELAY_URL","https://example.org/relay")
 t.Setenv("RELAY_SECRET",secret)
 dir:=t.TempDir()
 db,err:=storage.Open(filepath.Join(dir,"test.db"));if err!=nil{t.Fatal(err)}
 defer db.Close()
 sender,err:=New(db,dir);if err!=nil{t.Fatal(err)}
 if !sender.UsingRelay(){t.Fatal("expected relay mode")}
 sender.relayURL=relay.URL
 sender.client=relay.Client()
 if err:=sender.Send(context.Background(),storage.TelegramSettings{},"monitor down");err!=nil{t.Fatal(err)}
 if calls!=1{t.Fatalf("expected 1 relay call, got %d",calls)}
}
func TestRelayRejectsFailureAndRedirect(t *testing.T) {
 secret:="relay-secret"
 relay:=httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Location","https://external.example/")
  w.WriteHeader(http.StatusFound)
 }))
 defer relay.Close()
 t.Setenv("RELAY_URL","https://example.org/relay")
 t.Setenv("RELAY_SECRET",secret)
 dir:=t.TempDir()
 db,err:=storage.Open(filepath.Join(dir,"test.db"));if err!=nil{t.Fatal(err)}
 defer db.Close()
 sender,err:=New(db,dir);if err!=nil{t.Fatal(err)}
 sender.relayURL=relay.URL
 sender.client=relay.Client()
 err=sender.Send(context.Background(),storage.TelegramSettings{},"hello")
 if err==nil || !strings.Contains(err.Error(),"Relay HTTP 302"){t.Fatalf("expected non-followed redirect error, got %v",err)}
 if strings.Contains(err.Error(),secret){t.Fatal("relay secret leaked into error")}
}
func TestRelayRequiresBothVariables(t *testing.T) {
 t.Setenv("RELAY_URL","https://example.org/")
 t.Setenv("RELAY_SECRET","")
 dir:=t.TempDir()
 db,err:=storage.Open(filepath.Join(dir,"test.db"));if err!=nil{t.Fatal(err)}
 defer db.Close()
 if _,err:=New(db,dir);err==nil{t.Fatal("expected error with missing RELAY_SECRET")}
}
