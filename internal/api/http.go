// Package api provides a read-only, bearer-token protected REST API.
package api

import (
 "database/sql"
 "encoding/json"
 "errors"
 "net/http"
 "strconv"
 "strings"

 "example.com/site-observatory/internal/observatory"
 "example.com/site-observatory/internal/storage"
)

type Handler struct{db *storage.DB;svc *observatory.Service}
func New(db *storage.DB)http.Handler{
 h:=&Handler{db:db,svc:observatory.New(db)}
 mux:=http.NewServeMux()
 mux.HandleFunc("GET /api/v1/summary",h.summary)
 mux.HandleFunc("GET /api/v1/monitors",h.monitors)
 mux.HandleFunc("GET /api/v1/monitors/{id}",h.monitor)
 mux.HandleFunc("GET /api/v1/incidents",h.incidents)
 mux.HandleFunc("GET /api/v1/domains",h.domains)
 mux.HandleFunc("GET /api/v1/monitors/{id}/seo-issues",h.seo)
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Cache-Control","no-store")
  w.Header().Set("X-Content-Type-Options","nosniff")
  w.Header().Set("Referrer-Policy","no-referrer")
  // Never accept API credentials in query strings or cookies.
  value:=r.Header.Get("Authorization")
  if !strings.HasPrefix(value,"Bearer ") || !db.ValidAPIKey(strings.TrimPrefix(value,"Bearer ")) {
   w.Header().Set("WWW-Authenticate",`Bearer realm="Site Observatory API"`)
   sendError(w,http.StatusUnauthorized,"invalid_api_key")
   return
  }
  mux.ServeHTTP(w,r)
 })
}
func sendJSON(w http.ResponseWriter,status int,data any){
 w.Header().Set("Content-Type","application/json; charset=utf-8")
 w.WriteHeader(status)
 _=json.NewEncoder(w).Encode(data)
}
func sendError(w http.ResponseWriter,status int,code string){sendJSON(w,status,map[string]string{"error":code})}
func internal(w http.ResponseWriter){sendError(w,500,"internal_error")}
func parseID(w http.ResponseWriter,r *http.Request)(int64,bool){
 id,e:=strconv.ParseInt(r.PathValue("id"),10,64)
 if e!=nil||id<=0{sendError(w,400,"invalid_monitor_id");return 0,false}
 return id,true
}
func limit(w http.ResponseWriter,r *http.Request)(int,bool){
 raw:=r.URL.Query().Get("limit")
 if raw==""{return 25,true}
 n,e:=strconv.Atoi(raw)
 if e!=nil||n<1||n>100{sendError(w,400,"invalid_limit");return 0,false}
 return n,true
}
func(h *Handler) summary(w http.ResponseWriter,r *http.Request){
 value,e:=h.svc.Summary();if e!=nil{internal(w);return}
 sendJSON(w,200,value)
}
func(h *Handler) monitors(w http.ResponseWriter,r *http.Request){
 values,e:=h.svc.Monitors();if e!=nil{internal(w);return}
 sendJSON(w,200,map[string]any{"monitors":values})
}
func(h *Handler) monitor(w http.ResponseWriter,r *http.Request){
 id,ok:=parseID(w,r);if !ok{return}
 value,e:=h.svc.MonitorDetail(id)
 if errors.Is(e,sql.ErrNoRows){sendError(w,404,"monitor_not_found");return}
 if e!=nil{internal(w);return}
 sendJSON(w,200,value)
}
func(h *Handler) incidents(w http.ResponseWriter,r *http.Request){
 n,ok:=limit(w,r);if !ok{return}
 id:=int64(0)
 if raw:=r.URL.Query().Get("monitor_id");raw!=""{
  var e error
  id,e=strconv.ParseInt(raw,10,64)
  if e!=nil||id<=0{sendError(w,400,"invalid_monitor_id");return}
 }
 values,e:=h.svc.Incidents(id,n)
 if errors.Is(e,sql.ErrNoRows){sendError(w,404,"monitor_not_found");return}
 if e!=nil{internal(w);return}
 sendJSON(w,200,map[string]any{"incidents":values})
}
func(h *Handler) domains(w http.ResponseWriter,r *http.Request){
 values,e:=h.svc.Domains();if e!=nil{internal(w);return}
 sendJSON(w,200,map[string]any{"domains":values})
}
func(h *Handler) seo(w http.ResponseWriter,r *http.Request){
 id,ok:=parseID(w,r);if !ok{return}
 n,ok:=limit(w,r);if !ok{return}
 values,e:=h.svc.SEOIssues(id,n)
 if errors.Is(e,sql.ErrNoRows){sendError(w,404,"monitor_not_found");return}
 if e!=nil{internal(w);return}
 sendJSON(w,200,map[string]any{"issues":values})
}
