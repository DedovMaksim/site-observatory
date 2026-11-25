// Package observatory provides read-only views shared by REST and MCP transports.
package observatory

import (
 "database/sql"
 "errors"
 "strings"
 "time"

 "example.com/site-observatory/internal/storage"
)

type Service struct {DB *storage.DB}
func New(db *storage.DB)*Service{return &Service{DB:db}}

type Monitor struct {
 ID int64 `json:"id"`
 Name string `json:"name"`
 URL string `json:"url"`
 Status string `json:"status"`
 Enabled bool `json:"enabled"`
 LastCode int `json:"last_code"`
 LastLatencyMS int64 `json:"last_latency_ms"`
 LastChecked string `json:"last_checked"`
 LastError string `json:"last_error"`
}
func presentMonitor(m storage.Monitor)Monitor{
 return Monitor{ID:m.ID,Name:m.Name,URL:m.URL,Status:m.Status,Enabled:m.Enabled,LastCode:m.LastCode,LastLatencyMS:m.LastLatency,LastChecked:m.LastChecked,LastError:m.LastError}
}
func(s *Service) Monitors()([]Monitor,error){
 list,err:=s.DB.Monitors();if err!=nil{return nil,err}
 out:=make([]Monitor,0,len(list))
 for _,m:=range list{out=append(out,presentMonitor(m))}
 return out,nil
}
type Summary struct {
 Total int `json:"total"`
 Up int `json:"up"`
 Down int `json:"down"`
 Unknown int `json:"unknown"`
 OpenIncidents int `json:"open_incidents"`
}
func(s *Service) Summary()(Summary,error){
 monitors,err:=s.DB.Monitors();if err!=nil{return Summary{},err}
 result:=Summary{Total:len(monitors)}
 for _,m:=range monitors{switch m.Status{case "UP":result.Up++;case "DOWN":result.Down++;default:result.Unknown++}}
 err=s.DB.QueryRow(`SELECT COUNT(*) FROM incidents WHERE ended_at=''`).Scan(&result.OpenIncidents)
 return result,err
}
type Check struct{
 CheckedAt string `json:"checked_at"`
 HTTPCode int `json:"http_code"`
 LatencyMS int64 `json:"latency_ms"`
 Success bool `json:"success"`
 Error string `json:"error"`
}
type Incident struct{
 ID int64 `json:"id"`
 MonitorID int64 `json:"monitor_id"`
 StartedAt string `json:"started_at"`
 EndedAt string `json:"ended_at"`
 Reason string `json:"reason"`
}
func(s *Service) Incidents(monitorID int64,limit int)([]Incident,error){
 if limit<1||limit>100{return nil,errors.New("limit must be 1..100")}
 if monitorID!=0 {if _,err:=s.DB.Monitor(monitorID);err!=nil{return nil,err}}
 rows,err:=s.DB.Incidents(monitorID,limit);if err!=nil{return nil,err}
 out:=make([]Incident,0,len(rows))
 for _,x:=range rows{out=append(out,Incident{ID:x.ID,MonitorID:x.MonitorID,StartedAt:x.StartedAt,EndedAt:x.EndedAt,Reason:x.Reason})}
 return out,nil
}
type Diagnostic struct{
 CheckedAt string `json:"checked_at"`
 TLSStatus string `json:"tls_status"`
 TLSDays int `json:"tls_days_remaining"`
 TLSExpiry string `json:"tls_expiry"`
 DNSAddresses string `json:"dns_addresses"`
 DNSChanged bool `json:"dns_changed"`
 Title string `json:"title"`
 Canonical string `json:"canonical"`
 Robots string `json:"robots"`
 SEOStatus string `json:"seo_status"`
 Error string `json:"error"`
}
type Domain struct{
 Name string `json:"name"`
 ExpiryDate string `json:"expiry_date"`
 DaysRemaining int `json:"days_remaining"`
 Source string `json:"source"`
 HasExpiry bool `json:"has_expiry"`
 LastChecked string `json:"last_checked"`
 LastError string `json:"last_error"`
}
type Sitemap struct{
 Enabled bool `json:"enabled"`
 URL string `json:"url"`
 Status string `json:"status"`
 TotalURLs int `json:"total_urls"`
 Added int `json:"added"`
 Removed int `json:"removed"`
 XMLFiles int `json:"xml_files"`
 LastChecked string `json:"last_checked"`
 Error string `json:"error"`
}
type Robots struct{
 Enabled bool `json:"enabled"`
 Status string `json:"status"`
 LastChecked string `json:"last_checked"`
 RootBlocks string `json:"root_blocks"`
 SitemapDirectives string `json:"sitemap_directives"`
 Error string `json:"error"`
}
type PageAudit struct{
 Enabled bool `json:"enabled"`
 Total int `json:"total"`
 Checked int `json:"checked"`
 HTTPErrors int `json:"http_errors"`
 Noindex int `json:"noindex"`
 MissingTitle int `json:"missing_title"`
 MissingCanonical int `json:"missing_canonical"`
 OtherCanonical int `json:"other_canonical"`
 PendingAlerts int `json:"pending_alerts"`
 ActiveAlerts int `json:"active_alerts"`
}
type MonitorDetail struct{
 Monitor Monitor `json:"monitor"`
 Checks []Check `json:"checks"`
 Incidents []Incident `json:"incidents"`
 Uptime24h storage.UptimeStats `json:"uptime_24h"`
 Diagnostic *Diagnostic `json:"diagnostic,omitempty"`
 Domain *Domain `json:"domain,omitempty"`
 Sitemap *Sitemap `json:"sitemap,omitempty"`
 Robots *Robots `json:"robots,omitempty"`
 PageAudit PageAudit `json:"page_audit"`
}
func(s *Service) MonitorDetail(id int64)(MonitorDetail,error){
 m,err:=s.DB.Monitor(id);if err!=nil{return MonitorDetail{},err}
 v:=MonitorDetail{Monitor:presentMonitor(m),Checks:[]Check{},Incidents:[]Incident{}}
 checks,err:=s.DB.Checks(id,20);if err!=nil{return v,err}
 for _,c:=range checks{v.Checks=append(v.Checks,Check{CheckedAt:c.CheckedAt,HTTPCode:c.Code,LatencyMS:c.Latency,Success:c.Success,Error:c.Error})}
 v.Incidents,err=s.Incidents(id,20);if err!=nil{return v,err}
 v.Uptime24h,err=s.DB.Uptime(id,time.Now().Add(-24*time.Hour));if err!=nil{return v,err}
 d,err:=s.DB.Diagnostic(id)
 if err==nil{v.Diagnostic=&Diagnostic{CheckedAt:d.CheckedAt,TLSStatus:d.TLSStatus,TLSDays:d.TLSDays,TLSExpiry:d.TLSExpiry,DNSAddresses:d.DNSSummary,DNSChanged:d.DNSChanged,Title:d.Title,Canonical:d.Canonical,Robots:d.Robots,SEOStatus:d.SEOStatus,Error:d.Error}}else if !errors.Is(err,sql.ErrNoRows){return v,err}
 domain,err:=s.DB.DomainForMonitor(id)
 if err==nil{v.Domain=&Domain{Name:domain.Domain,ExpiryDate:domain.EffectiveDate,DaysRemaining:domain.DaysLeft,Source:domain.Source,HasExpiry:domain.HasDate,LastChecked:domain.LastChecked,LastError:domain.LastError}}else if !errors.Is(err,sql.ErrNoRows){return v,err}
 sitemap,err:=s.DB.SitemapForMonitor(id)
 if err==nil{v.Sitemap=&Sitemap{Enabled:sitemap.Enabled,URL:sitemap.URL,Status:sitemap.Status,TotalURLs:sitemap.Total,Added:sitemap.Added,Removed:sitemap.Removed,XMLFiles:sitemap.Files,LastChecked:sitemap.LastChecked,Error:sitemap.Error}}else if !errors.Is(err,sql.ErrNoRows){return v,err}
 robots,err:=s.DB.RobotsForMonitor(id)
 if err==nil{v.Robots=&Robots{Enabled:robots.Enabled,Status:robots.Status,LastChecked:robots.LastChecked,RootBlocks:robots.RootBlocks,SitemapDirectives:robots.SitemapList,Error:robots.LastError}}else if !errors.Is(err,sql.ErrNoRows){return v,err}
 config,err:=s.DB.PageAuditConfigs();if err!=nil{return v,err}
 for _,c:=range config{if c.MonitorID==id{v.PageAudit=PageAudit{Enabled:c.Enabled,Total:c.Total,Checked:c.Checked,HTTPErrors:c.Errors,Noindex:c.Noindex};break}}
 issues,err:=s.DB.PageAuditIssueCounts(id);if err!=nil{return v,err}
 v.PageAudit.MissingTitle=issues.Title;v.PageAudit.MissingCanonical=issues.Canonical;v.PageAudit.OtherCanonical=issues.CanonicalMismatch
 v.PageAudit.PendingAlerts,v.PageAudit.ActiveAlerts,err=s.DB.PageAuditAlertSummary(id)
 return v,err
}
type SEOIssue struct{
 URL string `json:"url"`
 HTTPStatus int `json:"http_status"`
 Title string `json:"title"`
 Canonical string `json:"canonical"`
 Noindex bool `json:"noindex"`
 MissingTitle bool `json:"missing_title"`
 MissingCanonical bool `json:"missing_canonical"`
 CanonicalMismatch bool `json:"canonical_mismatch"`
 Error string `json:"error"`
 CheckedAt string `json:"checked_at"`
}
func(s *Service) SEOIssues(id int64,limit int)([]SEOIssue,error){
 if limit<1||limit>100{return nil,errors.New("limit must be 1..100")}
 if _,err:=s.DB.Monitor(id);err!=nil{return nil,err}
 // Query pages with any issue; no raw HTML or private credentials exposed.
 rows,err:=s.DB.Query(`SELECT p.url,p.http_status,p.title,p.canonical,p.noindex,p.last_error,p.checked_at
 FROM page_audit_results p JOIN sitemap_urls u ON p.monitor_id=u.monitor_id AND p.url=u.url
 WHERE p.monitor_id=? AND (p.http_status=0 OR p.http_status>=400 OR p.last_error<>'' OR p.noindex=1 OR TRIM(p.title)='' OR TRIM(p.canonical)='' OR (TRIM(p.canonical)<>'' AND LOWER(RTRIM(p.canonical,'/'))<>LOWER(RTRIM(p.url,'/'))))
 ORDER BY p.checked_at DESC LIMIT ?`,id,limit)
 if err!=nil{return nil,err}
 defer rows.Close()
 out:=[]SEOIssue{}
 for rows.Next(){
  var x SEOIssue;var noindex int
  if err=rows.Scan(&x.URL,&x.HTTPStatus,&x.Title,&x.Canonical,&noindex,&x.Error,&x.CheckedAt);err!=nil{return nil,err}
  x.Noindex=noindex==1;x.MissingTitle=x.Title=="";x.MissingCanonical=x.Canonical==""
  x.CanonicalMismatch=!x.MissingCanonical&& !strings.EqualFold(strings.TrimRight(x.Canonical,"/"),strings.TrimRight(x.URL,"/"))
  out=append(out,x)
 }
 return out,rows.Err()
}
func(s *Service) Domains()([]Domain,error){
 domains,err:=s.DB.Domains();if err!=nil{return nil,err}
 out:=make([]Domain,0,len(domains))
 for _,d:=range domains{out=append(out,Domain{Name:d.Domain,ExpiryDate:d.EffectiveDate,DaysRemaining:d.DaysLeft,Source:d.Source,HasExpiry:d.HasDate,LastChecked:d.LastChecked,LastError:d.LastError})}
 return out,nil
}
