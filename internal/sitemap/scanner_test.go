package sitemap

import (
 "context"
 "fmt"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

func TestNestedIndexAndDeduplication(t *testing.T){
 mux:=http.NewServeMux()
 server:=httptest.NewServer(mux);defer server.Close()
 base:=server.URL
 mux.HandleFunc("/sitemap.xml",func(w http.ResponseWriter,r *http.Request){
  fmt.Fprintf(w,`<?xml version="1.0"?><sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><sitemap><loc>%s/one.xml</loc></sitemap><sitemap><loc>%s/two.xml</loc></sitemap></sitemapindex>`,base,base)
 })
 mux.HandleFunc("/one.xml",func(w http.ResponseWriter,r *http.Request){fmt.Fprintf(w,`<urlset><url><loc>%s/one</loc></url><url><loc>%s/shared</loc></url></urlset>`,base,base)})
 mux.HandleFunc("/two.xml",func(w http.ResponseWriter,r *http.Request){fmt.Fprintf(w,`<urlset><url><loc>%s/two</loc></url><url><loc>%s/shared</loc></url></urlset>`,base,base)})
 result,err:=(Scanner{Client:server.Client()}).Scan(context.Background(),base+"/",base+"/sitemap.xml")
 if err!=nil{t.Fatal(err)}
 if result.Files!=3||len(result.URLs)!=3 {t.Fatalf("got %+v",result)}
 if result.URLs[0]!=base+"/one" || result.URLs[1]!=base+"/shared"||result.URLs[2]!=base+"/two"{t.Fatalf("wrong URL list: %+v",result.URLs)}
}
func TestBlocksForeignListedURL(t *testing.T){
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){fmt.Fprint(w,`<urlset><url><loc>http://127.0.0.1/private</loc></url></urlset>`)}))
 defer server.Close()
 _,err:=(Scanner{Client:server.Client()}).Scan(context.Background(),server.URL,server.URL+"/sitemap.xml")
 if err==nil||!strings.Contains(err.Error(),"unsafe sitemap page URL"){t.Fatalf("expected block: %v",err)}
}
func TestRejectInvalidRoot(t *testing.T){
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){fmt.Fprint(w,`<html><p>not sitemap</p></html>`)}));defer server.Close()
 _,err:=(Scanner{Client:server.Client()}).Scan(context.Background(),server.URL,server.URL+"/sitemap.xml")
 if err==nil{t.Fatal("expected invalid root")}
}
