package web

import (
 "database/sql"
 "context"
 "fmt"
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

 
// domainTestTelegram sends an explicitly labelled sample through the real
// configured bot. It does NOT alter domain dates, outbox or alert deduplication.
func (s *Server) domainTestTelegram(w http.ResponseWriter,r *http.Request) {
 name:=strings.ToLower(strings.TrimSpace(r.PathValue("name")))
 domain,err:=s.DB.Domain(name)
 if errors.Is(err,sql.ErrNoRows) || domain.MonitorCount==0 {http.NotFound(w,r);return}
 if err!=nil{http.Error(w,"database error",500);return}
 if !domain.HasDate{http.Error(w,"Для теста сначала нужна дата регистрации домена",400);return}
 cfg,err:=s.DB.GetTelegram()
 if err!=nil{http.Error(w,"database error",500);return}
 if !cfg.Enabled || cfg.ChatID=="" || cfg.EncryptedToken==""{
  s.renderDomainsMessage(w,r,"","Telegram отключён или не настроен; откройте настройки",http.StatusBadRequest)
  return
 }
 message:=fmt.Sprintf("🧪 ТЕСТ Site Observatory — уведомление о регистрации домена\nДомен: %s\nДата окончания: %s\nСейчас осталось: %d дн.\nПример автоматического предупреждения за 30 дней.\n⚠️ Это тест, реальная дата не изменена.",domain.Domain,domain.EffectiveDate,domain.DaysLeft)
 ctx,cancel:=context.WithTimeout(r.Context(),12*time.Second)
 defer cancel()
 if err=s.Notify.Send(ctx,cfg,message);err!=nil{
  s.renderDomainsMessage(w,r,"",fmt.Sprintf("Тест Telegram не отправлен: %v",err),http.StatusBadGateway)
  return
 }
 s.renderDomainsMessage(w,r,"Пробное уведомление о "+domain.Domain+" отправлено в Telegram","",http.StatusOK)
}
func (s *Server) renderDomainsMessage(w http.ResponseWriter,r *http.Request,message,failure string,status int) {
 list,err:=s.DB.Domains()
 if err!=nil{http.Error(w,"database error",500);return}
 if status!=http.StatusOK{w.WriteHeader(status)}
 s.render(w,r,"domains.html",view{Title:"Сроки регистрации доменов",Domains:list,Message:message,Error:failure})
}
