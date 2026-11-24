package domains

import (
 "context"
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)
func TestRegisteredDomain(t *testing.T){
 cases:=map[string]string{
  "https://www.masaladabba.ru/spices":"masaladabba.ru",
  "https://shop.example.com/index":"example.com",
  "https://app.example.co.uk/path":"example.co.uk",
  "https://example.com:8443/":"example.com",
 }
 for input,want:=range cases{
  got,err:=RegisteredDomain(input)
  if err!=nil||got!=want{t.Fatalf("%s: got %q, error %v; want %s",input,got,err,want)}
 }
 for _,input:=range []string{"https://127.0.0.1","http://localhost","http://example.local","ftp://example.com"}{
  if _,err:=RegisteredDomain(input);err==nil{t.Fatalf("should reject %s",input)}
 }
}
func TestExpiryEvents(t *testing.T){
 events:=[]Event{{Action:"expiration",Date:"2028-01-01T00:00:00Z"},{Action:"registrar expiration",Date:"2027-12-20T00:00:00Z"}}
 date,kind,err:=ExpiryFromEvents(events)
 if err!=nil||kind!="registrar expiration"||date.Format("2006-01-02")!="2027-12-20"{t.Fatalf("bad expiry %v %s %v",date,kind,err)}
}
func TestBootstrapOnlyMatchingTLD(t *testing.T){
 registry:=Registry{Services:[][][]string{
  {{"com"},{"https://registry.com/"}},
  {{"org"},{"https://registry.org/"}},
 }}
 got,err:=baseForDomain("example.org",registry)
 if err!=nil||got!="https://registry.org"{t.Fatalf("wrong bootstrap base %q: %v",got,err)}
 if _,err:=baseForDomain("example.ru",registry);err==nil{t.Fatal("unlisted zone should be unsupported")}
}
func TestRDAPLookup(t *testing.T){
 mux:=http.NewServeMux()
 server:=httptest.NewServer(mux)
 defer server.Close()
 mux.HandleFunc("/bootstrap",func(w http.ResponseWriter,r *http.Request){
  json.NewEncoder(w).Encode(Registry{Services:[][][]string{{{"com"},{"https://example.org/"}}}})
 })
 mux.HandleFunc("/domain/example.com",func(w http.ResponseWriter,r *http.Request){
  json.NewEncoder(w).Encode(Registration{Events:[]Event{{Action:"expiration",Date:"2027-10-15T00:00:00Z"}}})
 })
 client:=NewClient()
 client.HTTP=server.Client()
 client.BootstrapURL=server.URL+"/bootstrap"
 // The base is read from IANA, so mock its hostname via the test transport.
 client.HTTP.Transport=roundTripFunc(func(req *http.Request)(*http.Response,error){
  if strings.HasPrefix(req.URL.String(),"https://example.org/domain/"){req.URL.Scheme="http";req.URL.Host=strings.TrimPrefix(server.URL,"http://")}
  return http.DefaultTransport.RoundTrip(req)
 })
 result,err:=client.Lookup(context.Background(),"example.com")
 if err!=nil||result.Expires.Format("2006-01-02")!="2027-10-15"{t.Fatalf("lookup %v %v",result,err)}
}
type roundTripFunc func(*http.Request)(*http.Response,error)
func (f roundTripFunc)RoundTrip(r *http.Request)(*http.Response,error){return f(r)}
