package storage

import (
 "crypto/rand"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "strings"
 "time"
)

func(d *DB) MigrateAPIKeys()error{
 _,err:=d.Exec(`
 CREATE TABLE IF NOT EXISTS api_keys(
 id INTEGER PRIMARY KEY,
 name TEXT NOT NULL UNIQUE,
 token_hash TEXT NOT NULL UNIQUE,
 created_at TEXT NOT NULL,
 revoked_at TEXT NOT NULL DEFAULT ''
 );
 CREATE INDEX IF NOT EXISTS idx_api_keys_hash ON api_keys(token_hash);
 `)
 return err
}
func (d *DB) CreateAPIKey(name string)(string,error){
 name=strings.TrimSpace(name)
 if len(name)<1||len(name)>80{return "",errors.New("key name must be 1..80 characters")}
 b:=make([]byte,32)
 if _,err:=rand.Read(b);err!=nil{return "",err}
 token:="so_ro_"+hex.EncodeToString(b)
 hash:=sha256.Sum256([]byte(token))
 _,err:=d.Exec(`INSERT INTO api_keys(name,token_hash,created_at) VALUES(?,?,?)`,name,hex.EncodeToString(hash[:]),time.Now().UTC().Format(time.RFC3339))
 if err!=nil{return "",err}
 return token,nil
}
func(d *DB) ValidAPIKey(token string)bool{
 if len(token)!=70||!strings.HasPrefix(token,"so_ro_"){return false}
 if _,err:=hex.DecodeString(token[len("so_ro_"):]);err!=nil{return false}
 hash:=sha256.Sum256([]byte(token))
 var ok int
 err:=d.QueryRow(`SELECT COUNT(*) FROM api_keys WHERE token_hash=? AND revoked_at=''`,hex.EncodeToString(hash[:])).Scan(&ok)
 return err==nil&&ok==1
}
func(d *DB) RevokeAPIKey(name string)error{
 result,err:=d.Exec(`UPDATE api_keys SET revoked_at=? WHERE name=? AND revoked_at=''`,time.Now().UTC().Format(time.RFC3339),name)
 if err!=nil{return err}
 n,err:=result.RowsAffected()
 if err!=nil{return err}
 if n==0{return errors.New("active API key not found")}
 return nil
}
