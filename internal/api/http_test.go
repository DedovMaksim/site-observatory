package api

import (
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "strings"
 "testing"

 "example.com/site-observatory/internal/storage"
)
func TestBearerOnlyRESTReadAPI(t *testing.T){
 db,err:=storage.Open(filepath.Join(t.TempDir(),"api.db"));if err!=nil{t.Fatal(err)}
 defer db.Close()
 id,err:=db.CreateMonitor("Demo","https://example.com",60,10,true);if err!=nil{t.Fatal(err)}
 if err=db.SyncMonitorDomain(id,"https://example.com");err!=nil{t.Fatal(err)}
 token,err:=db.CreateAPIKey("test-client");if err!=nil{t.Fatal(err)}
 server:=httptest.NewServer(New(db));defer server.Close()
 check:=func(path,authorization string,want int)map[string]any{
  t.Helper()
  req,err:=http.NewRequest("GET",server.URL+path,nil);if err!=nil{t.Fatal(err)}
  if authorization!=""{req.Header.Set("Authorization",authorization)}
  resp,err:=server.Client().Do(req);if err!=nil{t.Fatal(err)}
  defer resp.Body.Close()
  if resp.StatusCode!=want{t.Fatalf("%s got %d want %d",path,resp.StatusCode,want)}
  if got:=resp.Header.Get("Cache-Control");got!="no-store"{t.Fatalf("cache policy: %q",got)}
  var content map[string]any
  if err=json.NewDecoder(resp.Body).Decode(&content);err!=nil{t.Fatal(err)}
  return content
 }
 check("/api/v1/summary","",401)
 check("/api/v1/summary","Bearer not-valid",401)
 check("/api/v1/summary?token="+token,"",401)
 data:=check("/api/v1/summary","Bearer "+token,200)
 if data["total"].(float64)!=1{t.Fatalf("summary: %+v",data)}
 check("/api/v1/monitors","Bearer "+token,200)
 check("/api/v1/monitors/1","Bearer "+token,200)
 check("/api/v1/monitors/999999","Bearer "+token,404)
 check("/api/v1/incidents?limit=101","Bearer "+token,400)
 check("/api/v1/incidents","Bearer "+token,200)
 check("/api/v1/domains","Bearer "+token,200)
 check("/api/v1/monitors/1/seo-issues","Bearer "+token,200)
 // API keys never grant write endpoints or cookie-session access.
 req,err:=http.NewRequest("POST",server.URL+"/api/v1/monitors",strings.NewReader("{}"));if err!=nil{t.Fatal(err)}
 req.Header.Set("Authorization","Bearer "+token)
 resp,err:=server.Client().Do(req);if err!=nil{t.Fatal(err)}
 resp.Body.Close()
 if resp.StatusCode!=405{t.Fatalf("write unexpectedly accepted: %d",resp.StatusCode)}
 if err=db.RevokeAPIKey("test-client");err!=nil{t.Fatal(err)}
 check("/api/v1/summary","Bearer "+token,401)
}
