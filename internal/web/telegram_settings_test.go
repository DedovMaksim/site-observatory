package web

import (
 "html/template"
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
 // Verify the save path without rendering (the response uses the normal template).
 got,err:=s.telegramView("Settings","","")
 if err!=nil {t.Fatal(err)}
 tmpl,err:=template.New("base").Funcs(pageTemplateFuncs()).ParseFS(assets,"templates/*.html")
 if err!=nil {t.Fatal(err)}
 var out strings.Builder
 if err=tmpl.ExecuteTemplate(&out,"settings.html",got);err!=nil {t.Fatal(err)}
 if strings.Contains(out.String(),"name=\"bot_token\"") || strings.Contains(out.String(),"name=\"chat_id\"") {t.Fatal("direct credentials visible in relay mode")}
}
