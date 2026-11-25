package web

import (
 "database/sql"
 "errors"
 "net/http"
 "strconv"
 "example.com/site-observatory/internal/storage"
)
func(s *Server) pageAudits(w http.ResponseWriter,r *http.Request){
 configs,e:=s.DB.PageAuditConfigs()
 if e!=nil{http.Error(w,"database error",500);return}
 s.render(w,r,"pages.html",view{Title:"Проверка страниц sitemap",PageAuditConfigs:configs})
}
func(s *Server) pageAuditConfigure(w http.ResponseWriter,r *http.Request){
 id,e:=strconv.ParseInt(r.PathValue("id"),10,64)
 if e!=nil{http.NotFound(w,r);return}
 e=s.DB.ConfigurePageAudit(id,r.PostForm.Get("enabled")=="on")
 if errors.Is(e,sql.ErrNoRows){http.NotFound(w,r);return}
 if e!=nil{http.Error(w,"database error",500);return}
 http.Redirect(w,r,"/pages",http.StatusSeeOther)
}
func(s *Server) pageAuditDetail(w http.ResponseWriter,r *http.Request){
 id,e:=strconv.ParseInt(r.PathValue("id"),10,64)
 if e!=nil{http.NotFound(w,r);return}
 monitor,e:=s.DB.Monitor(id)
 if errors.Is(e,sql.ErrNoRows){http.NotFound(w,r);return}
 if e!=nil{http.Error(w,"database error",500);return}
 results,e:=s.DB.PageAuditResults(id,100)
 if e!=nil{http.Error(w,"database error",500);return}
 events,e:=s.DB.PageAuditEvents(id)
 if e!=nil{http.Error(w,"database error",500);return}
 s.render(w,r,"pages-detail.html",view{Title:"Аудит страниц",Monitor:monitor,PageAuditResults:results,PageAuditEvents:events})
}
var _ storage.PageAuditEvent
