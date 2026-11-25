package web

import (
 "database/sql"
 "errors"
 "net/http"
 "strconv"
)
func (s *Server) robotsList(w http.ResponseWriter,r *http.Request){
 list,e:=s.DB.RobotsList()
 if e!=nil{http.Error(w,"database error",500);return}
 s.render(w,r,"robots.html",view{Title:"Контроль robots.txt",RobotsList:list})
}
func (s *Server) robotsConfigure(w http.ResponseWriter,r *http.Request){
 id,e:=strconv.ParseInt(r.PathValue("id"),10,64)
 if e!=nil{http.NotFound(w,r);return}
 enabled:=r.PostForm.Get("enabled")=="on"
 e=s.DB.ConfigureRobots(id,enabled)
 if errors.Is(e,sql.ErrNoRows){http.NotFound(w,r);return}
 if e!=nil{http.Error(w,"database error",500);return}
 http.Redirect(w,r,"/robots",http.StatusSeeOther)
}
func (s *Server) robotsHistory(w http.ResponseWriter,r *http.Request){
 id,e:=strconv.ParseInt(r.PathValue("id"),10,64)
 if e!=nil{http.NotFound(w,r);return}
 state,e:=s.DB.RobotsForMonitor(id)
 if errors.Is(e,sql.ErrNoRows){http.NotFound(w,r);return}
 if e!=nil{http.Error(w,"database error",500);return}
 history,e:=s.DB.RobotsChanges(id)
 if e!=nil{http.Error(w,"database error",500);return}
 s.render(w,r,"robots-history.html",view{Title:"История robots.txt",Robots:state,RobotsChanges:history})
}
