// Package sitemap scans XML sitemap indexes without crawling the listed pages.
package sitemap

import (
 "compress/gzip"
 "context"
 "encoding/xml"
 "errors"
 "fmt"
 "io"
 "net"
 "net/http"
 "net/netip"
 "sort"
 "net/url"
 "strings"
 "time"
)

const (
 MaxSitemapFiles = 60
 MaxURLs = 20000
 MaxFileBytes = 12 << 20
)
type Result struct {
 URLs []string
 Files int
}
func DefaultURL(siteURL string)string {u,err:=url.Parse(siteURL);if err!=nil{return ""};return u.Scheme+"://"+u.Host+"/sitemap.xml"}
type Scanner struct {
 Client *http.Client
}
type xmlURL struct { Loc string `xml:"loc"` }
type sitemapXML struct {
 XMLName xml.Name
 URLs []xmlURL `xml:"url"`
 Maps []xmlURL `xml:"sitemap"`
}
func validateURL(raw string, host string)(*url.URL,error){
 u,err:=url.Parse(raw)
 if err!=nil || u.User!=nil || (u.Scheme!="https" && u.Scheme!="http") || u.Hostname()=="" || u.Fragment!="" || !strings.EqualFold(u.Hostname(),host) {
  return nil,errors.New("sitemap URL must use http(s) and the monitored hostname")
 }
 if u.Port()!="" && u.Port()!="443" && u.Port()!="80" {return nil,errors.New("custom sitemap port is not permitted")}
 return u,nil
}
func publicIP(ip net.IP)bool{
 addr,ok:=netip.AddrFromSlice(ip);if !ok{return false}
 addr=addr.Unmap()
 if !addr.IsGlobalUnicast()||addr.IsPrivate()||addr.IsLoopback()||addr.IsLinkLocalUnicast(){return false}
 if addr.Is4(){
  b:=addr.As4()
  if b[0]==0||b[0]==10||b[0]==127||b[0]>=224||b[0]==100&&b[1]>=64&&b[1]<=127||b[0]==169&&b[1]==254||b[0]==192&&b[1]==0&&b[2]==0||b[0]==198&&(b[1]==18||b[1]==19){return false}
 }
 return true
}
func newClient(host string)*http.Client{
 transport:=&http.Transport{
  DisableKeepAlives:true,DisableCompression:true,Proxy:nil,TLSHandshakeTimeout:7*time.Second,
  DialContext:func(ctx context.Context,network,address string)(net.Conn,error){
   h,p,err:=net.SplitHostPort(address);if err!=nil{return nil,err}
   if !strings.EqualFold(h,host){return nil,errors.New("blocked host")}
   ips,err:=net.DefaultResolver.LookupIP(ctx,"ip",h);if err!=nil{return nil,err}
   if len(ips)==0{return nil,errors.New("DNS has no addresses")}
   // Reject all private IPs, even when DNS mixes public and private answers.
   for _,ip:=range ips {if !publicIP(ip){return nil,errors.New("blocked DNS target")}}
   dialer:=&net.Dialer{Timeout:7*time.Second}
   var last error
   for _,ip:=range ips{
    conn,e:=dialer.DialContext(ctx,network,net.JoinHostPort(ip.String(),p))
    if e==nil{return conn,nil}
    last=e
   }
   return nil,last
  },
 }
 return &http.Client{
  Timeout:12*time.Second,Transport:transport,
  CheckRedirect:func(req *http.Request,via []*http.Request)error{
   if len(via)>4{return errors.New("too many sitemap redirects")}
   _,err:=validateURL(req.URL.String(),host);return err
  },
 }
}
func (s Scanner) Scan(ctx context.Context,siteURL,sitemapURL string)(Result,error){
 site,err:=url.Parse(siteURL);if err!=nil||site.Hostname()==""|| (site.Scheme!="https"&&site.Scheme!="http"){return Result{},errors.New("invalid monitored site URL")}
 host:=site.Hostname()
 if sitemapURL==""{sitemapURL=site.Scheme+"://"+site.Host+"/sitemap.xml"}
 if _,err:=validateURL(sitemapURL,host);err!=nil{return Result{},err}
 client:=s.Client
 if client==nil{client=newClient(host)}
 ctx,cancel:=context.WithTimeout(ctx,3*time.Minute);defer cancel()
 seenMaps:=make(map[string]bool)
 urls:=make(map[string]bool)
 pending:=[]string{sitemapURL}
 count:=0
 for len(pending)>0{
  if err:=ctx.Err();err!=nil{return Result{},err}
  raw:=pending[0];pending=pending[1:]
  if seenMaps[raw]{continue}
  if count>=MaxSitemapFiles{return Result{},fmt.Errorf("sitemap file limit exceeded (%d)",MaxSitemapFiles)}
  if _,err:=validateURL(raw,host);err!=nil{return Result{},err}
  seenMaps[raw]=true
  var parsed sitemapXML
  if err:=fetchXML(ctx,client,raw,&parsed);err!=nil{return Result{},fmt.Errorf("%s: %w",raw,err)}
  count++
  switch parsed.XMLName.Local{
  case "urlset":
   for _,entry:=range parsed.URLs{
    loc:=strings.TrimSpace(entry.Loc)
    // Sitemap URLs belong to the monitored hostname. Only collect,
    // never send HTTP requests to the listed pages.
    u,e:=validateURL(loc,host)
    if e!=nil{return Result{},fmt.Errorf("unsafe sitemap page URL: %w",e)}
    if u.String()!=loc{return Result{},errors.New("invalid sitemap URL encoding")}
    urls[loc]=true
    if len(urls)>MaxURLs{return Result{},fmt.Errorf("URL limit exceeded (%d)",MaxURLs)}
   }
  case "sitemapindex":
   for _,entry:=range parsed.Maps{
    loc:=strings.TrimSpace(entry.Loc)
    if _,e:=validateURL(loc,host);e!=nil{return Result{},fmt.Errorf("invalid nested sitemap: %w",e)}
    if !seenMaps[loc]{pending=append(pending,loc)}
    if len(pending)+len(seenMaps)>MaxSitemapFiles*2{return Result{},errors.New("sitemap index too large")}
   }
  default:return Result{},fmt.Errorf("unexpected sitemap XML element <%s>",parsed.XMLName.Local)
  }
 }
 out:=make([]string,0,len(urls))
 for u:=range urls{out=append(out,u)}
 // A stable order supports repeatable snapshots and tests.
 sort.Strings(out)
 return Result{URLs:out,Files:count},nil
}
func fetchXML(ctx context.Context,client *http.Client,raw string,into *sitemapXML)error{
 req,err:=http.NewRequestWithContext(ctx,"GET",raw,nil);if err!=nil{return err}
 req.Header.Set("User-Agent","SiteObservatory/0.5 (+sitemap-monitor)")
 resp,err:=client.Do(req);if err!=nil{return err}
 defer resp.Body.Close()
 if resp.StatusCode!=200{return fmt.Errorf("HTTP %d",resp.StatusCode)}
 var input io.Reader=io.LimitReader(resp.Body,MaxFileBytes+1)
 var zipped *gzip.Reader
 if strings.HasSuffix(strings.ToLower(resp.Request.URL.Path),".gz") || strings.Contains(strings.ToLower(resp.Header.Get("Content-Encoding")),"gzip"){
  zipped,err=gzip.NewReader(input);if err!=nil{return fmt.Errorf("gzip: %w",err)}
  defer zipped.Close()
  input=zipped
 }
 content,err:=io.ReadAll(io.LimitReader(input,MaxFileBytes+1))
 if err!=nil{return err}
 if len(content)>MaxFileBytes{return errors.New("sitemap XML exceeds size limit")}
 if err:=xml.Unmarshal(content,into);err!=nil{return fmt.Errorf("invalid sitemap XML: %w",err)}
 return nil
}

// FetchRobots reads only the fixed /robots.txt path on the monitored origin,
// using the same DNS, redirect and private-address protections as sitemap scans.
func FetchRobots(ctx context.Context,siteURL string)(string,error){
 site,err:=url.Parse(siteURL)
 if err!=nil||site.Hostname()==""||(site.Scheme!="https"&&site.Scheme!="http")||site.User!=nil{return "",errors.New("invalid monitored site")}
 target:=site.Scheme+"://"+site.Host+"/robots.txt"
 if _,err=validateURL(target,site.Hostname());err!=nil{return "",err}
 req,err:=http.NewRequestWithContext(ctx,"GET",target,nil);if err!=nil{return "",err}
 req.Header.Set("User-Agent","SiteObservatory/0.5 (+robots-monitor)")
 resp,err:=newClient(site.Hostname()).Do(req);if err!=nil{return "",err}
 defer resp.Body.Close()
 if resp.StatusCode!=200{return "",fmt.Errorf("robots.txt HTTP %d",resp.StatusCode)}
 const limit=512<<10
 data,err:=io.ReadAll(io.LimitReader(resp.Body,limit+1))
 if err!=nil{return "",err}
 if len(data)>limit{return "",errors.New("robots.txt exceeds 512 KiB limit")}
 return string(data),nil
}

// FetchPage reads a single page from a sitemap snapshot using the same
// strict same-host redirect and public-IP protections as sitemap scanning.
// HTTP error status codes are returned as data rather than transport errors.
func FetchPage(ctx context.Context,siteURL,pageURL string)(int,string,http.Header,error){
 site,err:=url.Parse(siteURL)
 if err!=nil||site.Hostname()==""||(site.Scheme!="http"&&site.Scheme!="https"){return 0,"",nil,errors.New("invalid monitored site URL")}
 if _,err=validateURL(pageURL,site.Hostname());err!=nil{return 0,"",nil,err}
 req,err:=http.NewRequestWithContext(ctx,"GET",pageURL,nil);if err!=nil{return 0,"",nil,err}
 req.Header.Set("User-Agent","SiteObservatory/0.6 (+page-audit)")
 resp,err:=newClient(site.Hostname()).Do(req)
 if err!=nil{return 0,"",nil,err}
 defer resp.Body.Close()
 // Non-HTML 2xx is recorded with the HTTP status but no SEO fields.
 if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")),"text/html"){
  return resp.StatusCode,"",resp.Header.Clone(),nil
 }
 const limit=2<<20
 content,err:=io.ReadAll(io.LimitReader(resp.Body,limit+1))
 if err!=nil{return resp.StatusCode,"",resp.Header.Clone(),err}
 if len(content)>limit{return resp.StatusCode,"",resp.Header.Clone(),errors.New("HTML response exceeds 2 MiB limit")}
 return resp.StatusCode,string(content),resp.Header.Clone(),nil
}
