package web

import (
 "database/sql"
 "errors"
 "net/http"
 "strings"
 "time"
)

func validDomainDate(s string)bool {
 if s=="" {return true}
 t,err:=time.Parse("2006-01-02",s)
 return err==nil&&t.Format("2006-01-02")==s
}
func (s *Server) domainList(w http.ResponseWriter,r *http.Request) {
 list,err:=s.DB.Domains()
 if err!=nil{http.Error(w,"database error",500);return}
 s.render(w,r,"domains.html",view{Title:"Сроки регистрации доменов",Domains:list})
}
func (s *Server) domainSaveManual(w http.ResponseWriter,r *http.Request) {
 name:=strings.ToLower(strings.TrimSpace(r.PathValue("name")))
 date:=strings.TrimSpace(r.PostForm.Get("manual_date"))
 if !validDomainDate(date){http.Error(w,"Неверная дата; используйте YYYY-MM-DD",400);return}
 if name==""||len(name)>253||strings.ContainsAny(name,"/\\?#%"){http.Error(w,"Неверное имя домена",400);return}
 err:=s.DB.SaveDomainManual(name,date)
 if errors.Is(err,sql.ErrNoRows){http.NotFound(w,r);return}
 if err!=nil{http.Error(w,"database error",500);return}
 http.Redirect(w,r,"/domains",http.StatusSeeOther)
}
