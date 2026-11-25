package web

import (
 "database/sql"
 "errors"
 "net/http"
 "strconv"
 "strings"
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
 filter:=r.URL.Query().Get("filter")
 if filter==""{filter="all"}
 if !storage.PageAuditFilterValid(filter){http.Error(w,"Некорректный фильтр",http.StatusBadRequest);return}
 search:=strings.TrimSpace(r.URL.Query().Get("q"))
 if len(search)>200{http.Error(w,"Поиск слишком длинный",http.StatusBadRequest);return}
 number,_:=strconv.Atoi(r.URL.Query().Get("page"))
 if number<1{number=1}
 if number>100000{http.Error(w,"Некорректная страница",http.StatusBadRequest);return}
 page,e:=s.DB.PageAuditFiltered(storage.PageAuditFilter{MonitorID:id,Search:search,Status:filter,Page:number,PageSize:25})
 if e!=nil{http.Error(w,"database error",500);return}
 configs,e:=s.DB.PageAuditConfigs()
 if e!=nil{http.Error(w,"database error",500);return}
 var config storage.PageAuditConfig
 for _,v:=range configs{if v.MonitorID==id{config=v;break}}
 issues,e:=s.DB.PageAuditIssueCounts(id)
 if e!=nil{http.Error(w,"database error",500);return}
 events,e:=s.DB.PageAuditEvents(id)
 if e!=nil{http.Error(w,"database error",500);return}
 v:=view{Title:"Аудит страниц",Monitor:monitor,PageAuditPage:page,PageAuditConfig:config,PageAuditIssues:issues,PageFilter:filter,PageSearch:search,PageAuditEvents:events}
 v.PageBefore=page.Page-1
 v.PageAfter=page.Page+1
 s.render(w,r,"pages-detail.html",v)
}
var _ storage.PageAuditEvent
