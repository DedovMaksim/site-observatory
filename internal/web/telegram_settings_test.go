package web

import (
 "html/template"
 "net/http/httptest"
 "net/url"
 "path/filepath"
 "strings"
 "testing"

 "example.com/site-observatory/internal/notify"
 "example.com/site-observatory/internal/storage"
)

func TestRelaySettingsHideDirectCredentials(t *testing.T) {
 t.Setenv("RELAY_URL","https://example.org/relay")
 t.Setenv("RELAY_SECRET","test-secret")
 dir:=t.TempDir()
 db,err:=storage.Open(filepath.Join(dir,"test.db"))
 if err!=nil {t.Fatal(err)}
 defer db.Close()
 n,err:=notify.New(db,dir)
 if err!=nil {t.Fatal(err)}
 token,err:=n.Encrypt("123:sample")
 if err!=nil {t.Fatal(err)}
 old:=storage.TelegramSettings{Enabled:true,ChatID:"-123",EncryptedToken:token}
 if err=db.SaveTelegram(old);err!=nil {t.Fatal(err)}
 s:=&Server{DB:db,Notify:n}
 got,err:=s.telegramView("Settings","","")
 if err!=nil {t.Fatal(err)}
 tmpl,err:=template.New("base").Funcs(pageTemplateFuncs()).ParseFS(assets,"templates/*.html")
 if err!=nil {t.Fatal(err)}
 var out strings.Builder
 if err=tmpl.ExecuteTemplate(&out,"settings.html",got);err!=nil {t.Fatal(err)}
 s.templates=tmpl
 req:=httptest.NewRequest("POST","/settings/telegram",nil)
 req.PostForm=url.Values{"enabled":{"on"},"notify_down":{"on"},"chat_id":{"invalid"},"bot_token":{"invalid"}}
 w:=httptest.NewRecorder()
 s.saveTelegram(w,req)
 if w.Code!=200 {t.Fatalf("save status %d",w.Code)}
 saved,err:=db.GetTelegram()
 if err!=nil {t.Fatal(err)}
 if saved.ChatID!=old.ChatID || saved.EncryptedToken!=old.EncryptedToken || !saved.NotifyDown {t.Fatal("relay save lost direct credentials or flags")}
 if strings.Contains(out.String(),"name=\"bot_token\"") || strings.Contains(out.String(),"name=\"chat_id\"") {t.Fatal("direct credentials visible in relay mode")}
}
