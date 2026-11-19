package notify

import (
 "path/filepath"
 "testing"
 "example.com/site-observatory/internal/storage"
)
func TestTokenEncryption(t *testing.T) {
 dir:=t.TempDir()
 db,err:=storage.Open(filepath.Join(dir,"test.db"));if err!=nil{t.Fatal(err)}
 defer db.Close()
 sender,err:=New(db,dir);if err!=nil{t.Fatal(err)}
 cipherText,err:=sender.Encrypt("123456:TEST-secret-token");if err!=nil{t.Fatal(err)}
 if cipherText=="123456:TEST-secret-token"{t.Fatal("plaintext token was stored")}
 got,err:=sender.decrypt(cipherText);if err!=nil{t.Fatal(err)}
 if got!="123456:TEST-secret-token"{t.Fatal("token roundtrip failed")}
 // Encryption must persist across restarts using the same private key file.
 restarted,err:=New(db,dir);if err!=nil{t.Fatal(err)}
 got,err=restarted.decrypt(cipherText);if err!=nil||got!="123456:TEST-secret-token"{t.Fatal("restart token decryption failed",err)}
}
