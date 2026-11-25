package storage

import (
 "path/filepath"
 "strings"
 "testing"
)
func TestAPIKeysCreateValidateAndRevoke(t *testing.T){
 db,err:=Open(filepath.Join(t.TempDir(),"api.db"));if err!=nil{t.Fatal(err)}
 defer db.Close()
 if db.ValidAPIKey(""){t.Fatal("empty token accepted")}
 token,err:=db.CreateAPIKey("local-agent")
 if err!=nil{t.Fatal(err)}
 if !strings.HasPrefix(token,"so_ro_")||len(token)!=70{t.Fatalf("wrong token format")}
 if !db.ValidAPIKey(token){t.Fatal("new key rejected")}
 if db.ValidAPIKey(token+"extra")||db.ValidAPIKey("so_ro_"+strings.Repeat("0",64)){t.Fatal("invalid key accepted")}
 if _,err=db.CreateAPIKey("local-agent");err==nil{t.Fatal("duplicate key name accepted")}
 var stored string
 if err=db.QueryRow(`SELECT token_hash FROM api_keys WHERE name='local-agent'`).Scan(&stored);err!=nil{t.Fatal(err)}
 if stored==token||strings.Contains(stored,"so_ro_"){t.Fatal("raw credential persisted")}
 if err=db.RevokeAPIKey("local-agent");err!=nil{t.Fatal(err)}
 if db.ValidAPIKey(token){t.Fatal("revoked key still accepted")}
 if err=db.RevokeAPIKey("local-agent");err==nil{t.Fatal("second revoke unexpectedly succeeded")}
}
