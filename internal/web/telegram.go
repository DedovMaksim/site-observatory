package web

import (
 "context"
 "fmt"
 "net/http"
 "regexp"
 "strings"
 "time"

 "example.com/site-observatory/internal/storage"
)

var telegramChatID = regexp.MustCompile(`^-?[0-9]{1,25}$`)

func (s *Server) telegramView(title, message, failure string) (view,error) {
 settings,err:=s.DB.GetTelegram()
 if err!=nil {return view{},err}
 return view{Title:title,Telegram:settings,TokenConfigured:settings.EncryptedToken!="",RelayEnabled:s.Notify.UsingRelay(),Message:message,Error:failure},nil
}
func (s *Server) settings(w http.ResponseWriter,r *http.Request) {
 v,err:=s.telegramView("Настройки Telegram","","")
 if err!=nil {http.Error(w,"database error",500);return}
 s.render(w,r,"settings.html",v)
}
func (s *Server) saveTelegram(w http.ResponseWriter,r *http.Request) {
 original,err:=s.DB.GetTelegram()
 if err!=nil {http.Error(w,"database error",500);return}
 original.Enabled=r.PostForm.Get("enabled")=="on"
 original.NotifyDown=r.PostForm.Get("notify_down")=="on"
 original.NotifyUp=r.PostForm.Get("notify_up")=="on"
 original.ChatID=strings.TrimSpace(r.PostForm.Get("chat_id"))
 token:=strings.TrimSpace(r.PostForm.Get("bot_token"))
 if token!="" {
  if !strings.Contains(token,":") || len(token)>256 {
   s.settingsError(w,r,"Неверный формат Bot Token");return
  }
  original.EncryptedToken,err=s.Notify.Encrypt(token)
  if err!=nil {http.Error(w,"unable to encrypt token",500);return}
 }
 if original.ChatID!="" && !telegramChatID.MatchString(original.ChatID) {
  s.settingsError(w,r,"Chat ID должен быть числом, возможно отрицательным");return
 }
 if original.Enabled && !s.Notify.UsingRelay() && (original.ChatID=="" || original.EncryptedToken=="") {
  s.settingsError(w,r,"Для включения уведомлений укажите Bot Token и Chat ID");return
 }
 if err=s.DB.SaveTelegram(original);err!=nil {http.Error(w,"database error",500);return}
 v,err:=s.telegramView("Настройки Telegram","Настройки сохранены","")
 if err!=nil {http.Error(w,"database error",500);return}
 s.render(w,r,"settings.html",v)
}
func (s *Server) settingsError(w http.ResponseWriter,r *http.Request,msg string) {
 v,err:=s.telegramView("Настройки Telegram","",msg)
 if err!=nil {http.Error(w,"database error",500);return}
 w.WriteHeader(http.StatusBadRequest)
 s.render(w,r,"settings.html",v)
}
func (s *Server) testTelegram(w http.ResponseWriter,r *http.Request) {
 ctx,cancel:=context.WithTimeout(r.Context(),12*time.Second);defer cancel()
 err:=s.Notify.Test(ctx)
 message,failure:="",""
 if err!=nil {failure=fmt.Sprintf("Тест не отправлен: %v",err)}else{message="Тестовое сообщение отправлено"}
 v,e:=s.telegramView("Настройки Telegram",message,failure)
 if e!=nil {http.Error(w,"database error",500);return}
 if err!=nil {w.WriteHeader(http.StatusBadGateway)}
 s.render(w,r,"settings.html",v)
}

// Ensure the Telegram configuration remains a server-side object, never returned by API.
var _ storage.TelegramSettings
