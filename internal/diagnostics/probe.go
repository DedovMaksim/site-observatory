// Package diagnostics provides bounded TLS, DNS and SEO checks independently of uptime.
package diagnostics

import (
 "context"
 "crypto/tls"
 "errors"
 "fmt"
 "io"
 "net"
 "net/http"
 "net/netip"
 "net/url"
 "regexp"
 "sort"
 "strings"
 "time"
)

type Report struct {
 CheckedAt string
 TLSStatus string
 TLSDays int
 TLSExpiry string
 DNSSummary string
 DNSChanged bool
 Title string
 Canonical string
 Robots string
 SEOStatus string
 Error string
}
var titleRE=regexp.MustCompile(`(?is)<title\b[^>]*>(.*?)</title\s*>`)
var headTagRE=regexp.MustCompile(`(?is)<(?:meta|link)\b[^>]*>`)
var attrRE=regexp.MustCompile(`(?i)([a-z][a-z0-9_-]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
var stripTags=regexp.MustCompile(`(?s)<[^>]*>`)

func attrs(tag string) map[string]string {
 a:=map[string]string{}
 for _,m:=range attrRE.FindAllStringSubmatch(tag,-1) {
  v:=m[2];if v=="" {v=m[3]};if v==""{v=m[4]}
  a[strings.ToLower(m[1])]=strings.TrimSpace(v)
 }
 return a
}
func ParseSEO(html string)(title,canonical,robots string) {
 if m:=titleRE.FindStringSubmatch(html);len(m)>1 {title=strings.TrimSpace(stripTags.ReplaceAllString(m[1],""))}
 for _,tag:=range headTagRE.FindAllString(html,-1){
  a:=attrs(tag)
  if strings.EqualFold(a["rel"],"canonical") && canonical=="" {canonical=a["href"]}
  if strings.EqualFold(a["name"],"robots") && robots=="" {robots=a["content"]}
 }
 if len(title)>500 {title=title[:500]}
 if len(canonical)>2000 {canonical=canonical[:2000]}
 if len(robots)>500 {robots=robots[:500]}
 return
}
func public(ip net.IP) bool{
 addr,ok:=netip.AddrFromSlice(ip);if !ok{return false}
 addr=addr.Unmap()
 if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast(){return false}
 if addr.Is4() {
  b:=addr.As4()
  if b[0]==0 || b[0]==10 || b[0]==127 || b[0]>=224 || b[0]==100 && b[1]>=64 && b[1]<=127 || b[0]==169&&b[1]==254 || b[0]==192&&b[1]==0&&b[2]==0 || b[0]==198&&(b[1]==18||b[1]==19){return false}
 }
 return true
}
func lookup(ctx context.Context, host string)([]net.IP,error){
 ips,err:=net.DefaultResolver.LookupIP(ctx,"ip",host)
 if err!=nil{return nil,err}
 if len(ips)==0{return nil,errors.New("DNS has no IP addresses")}
 for _,ip:=range ips {if !public(ip){return nil,errors.New("DNS returned blocked IP address")}}
 return ips,nil
}
func hostFrom(raw string)(*url.URL,error){
 u,err:=url.Parse(raw)
 if err!=nil || u.Hostname()=="" || u.User!=nil || (u.Scheme!="http"&&u.Scheme!="https"){
  return nil,errors.New("expected http(s) URL with hostname")
 }
 return u,nil
}
func Run(ctx context.Context,raw string,timeout int) Report {
 r:=Report{CheckedAt:time.Now().UTC().Format(time.RFC3339),TLSStatus:"N/A",SEOStatus:"UNKNOWN"}
 u,err:=hostFrom(raw)
 if err!=nil {r.Error=err.Error();return r}
 if timeout<1||timeout>60 {timeout=10}
 ctx,cancel:=context.WithTimeout(ctx,time.Duration(timeout+5)*time.Second);defer cancel()
 host:=u.Hostname()
 ips,err:=lookup(ctx,host)
 if err!=nil{r.Error=err.Error();return r}
 addresses:=make([]string,0,len(ips))
 for _,ip:=range ips {addresses=append(addresses,ip.String())}
 sort.Strings(addresses)
 r.DNSSummary=strings.Join(addresses,", ")
 // TLS validation uses a direct public IP with the original hostname as SNI.
 if u.Scheme=="https" {
  port:=u.Port();if port=="" {port="443"}
  d:=net.Dialer{Timeout:time.Duration(timeout)*time.Second}
  connection,e:=tls.DialWithDialer(&d,"tcp",net.JoinHostPort(ips[0].String(),port),&tls.Config{ServerName:host,MinVersion:tls.VersionTLS12})
  if e!=nil {r.TLSStatus="ERROR";r.Error="TLS: "+e.Error()} else {
   cert:=connection.ConnectionState().PeerCertificates[0]
   r.TLSExpiry=cert.NotAfter.UTC().Format(time.RFC3339)
   r.TLSDays=int(time.Until(cert.NotAfter).Hours()/24)
   r.TLSStatus="OK"
   if r.TLSDays<30 {r.TLSStatus="EXPIRING"}
   connection.Close()
  }
 }
 transport:=&http.Transport{Proxy:nil,DisableKeepAlives:true,TLSHandshakeTimeout:time.Duration(timeout)*time.Second}
 defer transport.CloseIdleConnections()
 transport.DialContext=func(ctx context.Context,network,address string)(net.Conn,error){
  h,p,e:=net.SplitHostPort(address);if e!=nil{return nil,e}
  safe,e:=lookup(ctx,h);if e!=nil{return nil,e}
  d:=net.Dialer{Timeout:time.Duration(timeout)*time.Second}
  var last error
  for _,ip:=range safe {conn,err:=d.DialContext(ctx,network,net.JoinHostPort(ip.String(),p));if err==nil{return conn,nil};last=err}
  return nil,last
 }
 client:=&http.Client{Transport:transport,Timeout:time.Duration(timeout)*time.Second,CheckRedirect:func(req *http.Request,via []*http.Request)error{
  if len(via)>=5{return errors.New("too many redirects")}
  _,err:=hostFrom(req.URL.String());return err
 }}
 req,e:=http.NewRequestWithContext(ctx,"GET",raw,nil)
 if e!=nil {r.SEOStatus="ERROR";return r}
 req.Header.Set("User-Agent","SiteObservatory/0.3")
 resp,e:=client.Do(req)
 if e!=nil {r.SEOStatus="ERROR";if r.Error==""{r.Error=e.Error()};return r}
 defer resp.Body.Close()
 if resp.StatusCode>=400 {r.SEOStatus="HTTP ERROR";return r}
 if ct:=resp.Header.Get("Content-Type");ct!=""&&!strings.Contains(strings.ToLower(ct),"html"){r.SEOStatus="NON-HTML";return r}
 content,e:=io.ReadAll(io.LimitReader(resp.Body,512*1024))
 if e!=nil{r.SEOStatus="ERROR";return r}
 r.Title,r.Canonical,r.Robots=ParseSEO(string(content))
 r.SEOStatus="OK"
 if r.Title=="" || strings.Contains(strings.ToLower(r.Robots),"noindex"){r.SEOStatus="WARNING"}
 return r
}
var _=fmt.Sprint
