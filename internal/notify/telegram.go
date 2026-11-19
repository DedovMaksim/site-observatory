package notify

import (
 "bytes"
 "context"
 "crypto/aes"
 "crypto/cipher"
 "crypto/rand"
 "encoding/base64"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "log"
 "net/http"
 "net/url"
 "os"
 "path/filepath"
 "strings"
 "time"

 "example.com/site-observatory/internal/storage"
)

type Telegram struct {
 DB *storage.DB
 key []byte
 client *http.Client
 apiURL string
}
func New(db *storage.DB, dataDir string) (*Telegram,error) {
 p:=filepath.Join(dataDir,"telegram.key")
 key,err:=os.ReadFile(p)
 if errors.Is(err,os.ErrNotExist) {
  key=make([]byte,32)
  if _,err=rand.Read(key);err!=nil{return nil,err}
  if err=os.WriteFile(p,key,0600);err!=nil{return nil,err}
 } else if err!=nil {return nil,err}
 if len(key)!=32{return nil,errors.New("invalid telegram.key: expected 32 bytes")}
 return &Telegram{DB:db,key:key,client:&http.Client{Timeout:10*time.Second},apiURL:"https://api.telegram.org"},nil
}
func (t *Telegram) Encrypt(token string)(string,error) {
 block,err:=aes.NewCipher(t.key);if err!=nil{return "",err}
 gcm,err:=cipher.NewGCM(block);if err!=nil{return "",err}
 nonce:=make([]byte,gcm.NonceSize())
 if _,err=rand.Read(nonce);err!=nil{return "",err}
 data:=gcm.Seal(nonce,nonce,[]byte(token),nil)
 return base64.RawStdEncoding.EncodeToString(data),nil
}
func (t *Telegram) decrypt(ciphertext string)(string,error) {
 raw,err:=base64.RawStdEncoding.DecodeString(ciphertext);if err!=nil{return "",errors.New("invalid encrypted token")}
 block,err:=aes.NewCipher(t.key);if err!=nil{return "",err}
 gcm,err:=cipher.NewGCM(block);if err!=nil{return "",err}
 if len(raw)<gcm.NonceSize(){return "",errors.New("invalid encrypted token")}
 plain,err:=gcm.Open(nil,raw[:gcm.NonceSize()],raw[gcm.NonceSize():],nil)
 if err!=nil{return "",errors.New("token decryption failed; restore telegram.key from backup")}
 return string(plain),nil
}
func (t *Telegram) Send(ctx context.Context,s storage.TelegramSettings,message string) error {
 if s.ChatID=="" || s.EncryptedToken=="" {return errors.New("configure Telegram token and Chat ID")}
 token,err:=t.decrypt(s.EncryptedToken);if err!=nil{return err}
 // The token is never included in the returned error or logs.
 endpoint:=t.apiURL+"/bot"+url.PathEscape(token)+"/sendMessage"
 body,err:=json.Marshal(map[string]string{"chat_id":s.ChatID,"text":message});if err!=nil{return err}
 req,err:=http.NewRequestWithContext(ctx,"POST",endpoint,bytes.NewReader(body));if err!=nil{return errors.New("Telegram request initialization failed")}
 req.Header.Set("Content-Type","application/json")
 res,err:=t.client.Do(req);if err!=nil{return errors.New("Telegram connection failed")}
 defer res.Body.Close()
 data,err:=io.ReadAll(io.LimitReader(res.Body,4096));if err!=nil{return errors.New("Telegram response read failed")}
 var out struct{Ok bool `json:"ok"`;Description string `json:"description"`}
 if json.Unmarshal(data,&out)!=nil{return errors.New("invalid Telegram response")}
 if !out.Ok || res.StatusCode!=200 {
  // Keep diagnostics concise; never expose the bot token.
  return fmt.Errorf("Telegram API HTTP %d: %s",res.StatusCode,strings.TrimSpace(out.Description))
 }
 return nil
}
func (t *Telegram) Test(ctx context.Context) error {
 s,err:=t.DB.GetTelegram();if err!=nil{return err}
 return t.Send(ctx,s,"✅ Site Observatory: тестовое уведомление успешно отправлено.")
}
func (t *Telegram) Run(ctx context.Context) {
 tick:=time.NewTicker(5*time.Second);defer tick.Stop()
 for {
  select {case <-ctx.Done():return;case <-tick.C:t.process(ctx)}
 }
}
func (t *Telegram) process(ctx context.Context) {
 s,err:=t.DB.GetTelegram();if err!=nil || !s.Enabled || s.EncryptedToken=="" || s.ChatID=="" {return}
 m,err:=t.DB.NextTelegramMessage(ctx);if err!=nil{return}
 sendCtx,cancel:=context.WithTimeout(ctx,12*time.Second)
 err=t.Send(sendCtx,s,m.Text)
 cancel()
 if err==nil {
  if e:=t.DB.MarkTelegramSent(ctx,m.ID);e!=nil{log.Printf("telegram outbox mark sent: %v",e)}
 }else{
  log.Printf("telegram delivery failed for message %d: %v",m.ID,err)
  if e:=t.DB.RetryTelegram(ctx,m.ID,m.Attempts+1);e!=nil{log.Printf("telegram retry: %v",e)}
 }
}
