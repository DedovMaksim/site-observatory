package web

import (
 "database/sql"
 "errors"
 "net/http"
 "net/url"
 "strconv"
 "strings"

 "example.com/site-observatory/internal/sitemap"
 "example.com/site-observatory/internal/storage"
)
func (s *Server) sitemapList(w http.ResponseWriter,r *http.Request){
 sites,err:=s.DB.SitemapList()
 if err!=nil{http.Error(w,"database error",500);return}
 s.render(w,r,"sitemaps.html",view{Title:"Sitemap-мониторинг",Sitemaps:sites})
}
func (s *Server) sitemapConfigure(w http.ResponseWriter,r *http.Request){
 id,err:=strconv.ParseInt(r.PathValue("id"),10,64)
 if err!=nil{http.NotFound(w,r);return}
 monitor,err:=s.DB.Monitor(id)
 if errors.Is(err,sql.ErrNoRows){http.NotFound(w,r);return}
 if err!=nil{http.Error(w,"database error",500);return}
 raw:=strings.TrimSpace(r.PostForm.Get("sitemap_url"))
 if len(raw)>2048{http.Error(w,"Sitemap URL слишком длинный",400);return}
 if raw!=""{
  site,err:=url.Parse(monitor.URL)
  if err!=nil{http.Error(w,"Некорректный URL монитора",400);return}
  parsed,err:=url.Parse(raw)
  if err!=nil||parsed.Hostname()==""||!strings.EqualFold(parsed.Hostname(),site.Hostname())||parsed.User!=nil||(parsed.Scheme!="https"&&parsed.Scheme!="http")||parsed.Fragment!=""||(parsed.Port()!=""&&parsed.Port()!="80"&&parsed.Port()!="443"){
   http.Error(w,"Sitemap должен быть HTTP(S) URL того же домена без нестандартного порта",400);return
  }
 }
 enabled:=r.PostForm.Get("enabled")=="on"
 if enabled&&raw==""{raw=sitemap.DefaultURL(monitor.URL)}
 if err=s.DB.ConfigureSitemap(id,enabled,raw);err!=nil{http.Error(w,"Не удалось сохранить sitemap",500);return}
 http.Redirect(w,r,"/sitemaps",http.StatusSeeOther)
}
func (s *Server) sitemapChanges(w http.ResponseWriter,r *http.Request){
 id,err:=strconv.ParseInt(r.PathValue("id"),10,64)
 if err!=nil{http.NotFound(w,r);return}
 state,err:=s.DB.SitemapForMonitor(id)
 if errors.Is(err,sql.ErrNoRows){http.NotFound(w,r);return}
 if err!=nil{http.Error(w,"database error",500);return}
 changes,err:=s.DB.SitemapChanges(id,100)
 if err!=nil{http.Error(w,"database error",500);return}
 s.render(w,r,"sitemap-changes.html",view{Title:"Изменения sitemap",Sitemap:state,SitemapChanges:changes})
}
var _ storage.SitemapChange
