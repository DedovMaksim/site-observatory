package sitemap

import (
 "context"
 "fmt"
 "net/http"
 "net/http/httptest"
 "net/url"
 "strings"
 "testing"
)

type roundTripFunc func(*http.Request)(*http.Response,error)
func (fn roundTripFunc)RoundTrip(r *http.Request)(*http.Response,error){return fn(r)}
func setupScanner(t *testing.T, handler http.Handler)(Scanner,string){
 t.Helper()
 server:=httptest.NewServer(handler)
 t.Cleanup(server.Close)
 target,_:=url.Parse(server.URL)
 client:=&http.Client{Transport:roundTripFunc(func(r *http.Request)(*http.Response,error){
  clone:=r.Clone(r.Context())
  u:=*clone.URL
  u.Scheme=target.Scheme
  u.Host=target.Host
  clone.URL=&u
  return http.DefaultTransport.RoundTrip(clone)
 })}
 return Scanner{Client:client},"https://example.com"
}
func TestNestedIndexAndDeduplication(t *testing.T){
 const base="https://example.com"
 mux:=http.NewServeMux()
 mux.HandleFunc("/sitemap.xml",func(w http.ResponseWriter,r *http.Request){
  fmt.Fprintf(w,`<?xml version="1.0"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><sitemap><loc>%s/one.xml</loc></sitemap><sitemap><loc>%s/two.xml</loc></sitemap></sitemapindex>`,base,base)
 })
 mux.HandleFunc("/one.xml",func(w http.ResponseWriter,r *http.Request){fmt.Fprintf(w,`<urlset><url><loc>%s/one</loc></url><url><loc>%s/shared</loc></url></urlset>`,base,base)})
 mux.HandleFunc("/two.xml",func(w http.ResponseWriter,r *http.Request){fmt.Fprintf(w,`<urlset><url><loc>%s/two</loc></url><url><loc>%s/shared</loc></url></urlset>`,base,base)})
 scanner,site:=setupScanner(t,mux)
 result,err:=scanner.Scan(context.Background(),site+"/",site+"/sitemap.xml")
 if err!=nil{t.Fatal(err)}
 if result.Files!=3||len(result.URLs)!=3{t.Fatalf("got %+v",result)}
 if result.URLs[0]!=base+"/one"||result.URLs[1]!=base+"/shared"||result.URLs[2]!=base+"/two"{t.Fatalf("wrong URL list: %+v",result.URLs)}
}
func TestBlocksForeignListedURL(t *testing.T){
 scanner,base:=setupScanner(t,http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){fmt.Fprint(w,`<urlset><url><loc>http://127.0.0.1/private</loc></url></urlset>`)}))
 _,err:=scanner.Scan(context.Background(),base,base+"/sitemap.xml")
 if err==nil||!strings.Contains(err.Error(),"unsafe sitemap page URL"){t.Fatalf("expected block: %v",err)}
}
func TestRejectInvalidRoot(t *testing.T){
 scanner,base:=setupScanner(t,http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){fmt.Fprint(w,`<html><p>not sitemap</p></html>`)}))
 _,err:=scanner.Scan(context.Background(),base,base+"/sitemap.xml")
 if err==nil{t.Fatal("expected invalid root")}
}
