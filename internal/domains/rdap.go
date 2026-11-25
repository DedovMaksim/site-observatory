package domains

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net"
 "net/http"
 "net/url"
 "sort"
 "strings"
 "sync"
 "time"
)

// RegisteredDomain derives the domain from a monitor URL. It supports common
// multi-label suffixes; see docs for the explicit suffix-list limitation.
func RegisteredDomain(raw string)(string,error){
 u,err:=url.Parse(raw);if err!=nil{return "",err}
 if u.Scheme!="https"&&u.Scheme!="http"{return "",errors.New("not an HTTP URL")}
 h:=strings.ToLower(strings.TrimSuffix(u.Hostname(),"."))
 if net.ParseIP(h)!=nil{return "",errors.New("IP address has no registration")}
 if h=="localhost"||strings.HasSuffix(h,".local"){return "",errors.New("local hostname")}
 labels:=strings.Split(h,".")
 if len(labels)<2{return "",errors.New("hostname has no registrable domain")}
 for _,s:=range labels{if len(s)<1||len(s)>63||s[0]=='-'||s[len(s)-1]=='-'{return "",errors.New("invalid domain label")};for _,ch:=range s{if (ch<'a'||ch>'z')&&(ch<'0'||ch>'9')&&ch!='-'{return "",errors.New("unsupported hostname; use ASCII/punycode")}}}
 if len(h)>253{return "",errors.New("hostname too long")}
 suffix:=strings.Join(labels[len(labels)-2:],".")
 multi:=map[string]bool{"co.uk":true,"org.uk":true,"ac.uk":true,"gov.uk":true,"com.au":true,"net.au":true,"org.au":true,"co.jp":true,"ne.jp":true,"or.jp":true,"com.br":true,"com.cn":true,"com.tr":true,"co.nz":true,"com.ua":true,"co.in":true,"com.mx":true}
 count:=2
 if multi[suffix]{count=3}
 if len(labels)<count{return "",errors.New("no registrable domain")}
 return strings.Join(labels[len(labels)-count:],"."),nil
}

type LookupResult struct {
 Expires time.Time
 Source string
}

type Registry struct {
 Services [][][]string `json:"services"`
}
type Event struct {
 Action string `json:"eventAction"`
 Date string `json:"eventDate"`
}
type Registration struct {
 Events []Event `json:"events"`
}
func ExpiryFromEvents(events []Event)(time.Time,string,error){
 for _,kind:=range []string{"registrar expiration","expiration"}{
  for _,e:=range events {if strings.EqualFold(strings.TrimSpace(e.Action),kind){
   d,err:=time.Parse(time.RFC3339,e.Date)
   if err!=nil {continue}
   return d.UTC(),kind,nil
  }}
 }
 return time.Time{},"",errors.New("RDAP did not return a registration expiration event")
}

type Client struct { HTTP *http.Client; BootstrapURL string; mu sync.Mutex; cached Registry; cachedAt time.Time }
func NewClient()*Client{
 return &Client{HTTP:&http.Client{Timeout:12*time.Second,CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}},BootstrapURL:"https://data.iana.org/rdap/dns.json"}
}
func readJSON(ctx context.Context,client *http.Client,endpoint string,max int64,v any)error{
 req,err:=http.NewRequestWithContext(ctx,"GET",endpoint,nil)
 if err!=nil{return err}
 req.Header.Set("Accept","application/rdap+json, application/json")
 response,err:=client.Do(req);if err!=nil{return err}
 defer response.Body.Close()
 if response.StatusCode!=200{return fmt.Errorf("HTTP %d",response.StatusCode)}
 data,err:=io.ReadAll(io.LimitReader(response.Body,max+1));if err!=nil{return err}
 if int64(len(data))>max{return errors.New("RDAP response too large")}
 return json.Unmarshal(data,v)
}
func baseForDomain(d string,r Registry)(string,error){
 labels:=strings.Split(d,".")
 for n:=len(labels);n>=1;n--{
  suffix:=strings.Join(labels[len(labels)-n:],".")
  for _,svc:=range r.Services {
   if len(svc)!=2 {continue}
   matched:=false
   for _,name:=range svc[0]{if strings.EqualFold(name,suffix){matched=true;break}}
   if !matched {continue}
   bases:=append([]string(nil),svc[1]...)
   sort.Strings(bases)
   for _,base:=range bases {
    parsed,err:=url.Parse(base)
    if err==nil&&parsed.Scheme=="https"&&parsed.Hostname()!=""&&parsed.User==nil&&parsed.Port()==""{
     return strings.TrimRight(base,"/"),nil
    }
   }
  }
 }
 return "",errors.New("RDAP is unavailable for this domain zone; enter expiry manually")
}
func (c *Client) Lookup(ctx context.Context,domain string)(LookupResult,error){
 if domain==""||strings.ContainsAny(domain,"/?:#@%\\") || strings.Contains(domain,".."){return LookupResult{},errors.New("invalid domain")}
 client:=c.HTTP;if client==nil{client=NewClient().HTTP}
 bootstrap:=c.BootstrapURL
 if bootstrap==""{bootstrap="https://data.iana.org/rdap/dns.json"}
 c.mu.Lock()
 registry:=c.cached
 if c.cachedAt.IsZero() || time.Since(c.cachedAt)>24*time.Hour {
  if err:=readJSON(ctx,client,bootstrap,1024*1024,&registry);err!=nil {c.mu.Unlock();return LookupResult{},fmt.Errorf("RDAP bootstrap: %w",err)}
  c.cached=registry
  c.cachedAt=time.Now()
 }
 c.mu.Unlock()
 base,err:=baseForDomain(domain,registry);if err!=nil{return LookupResult{},err}
 var r Registration
 endpoint:=base+"/domain/"+url.PathEscape(domain)
 if err=readJSON(ctx,client,endpoint,512*1024,&r);err!=nil{return LookupResult{},fmt.Errorf("RDAP domain lookup: %w",err)}
 expiry,kind,err:=ExpiryFromEvents(r.Events);if err!=nil{return LookupResult{},err}
 return LookupResult{Expires:expiry,Source:"RDAP ("+kind+")"},nil
}
