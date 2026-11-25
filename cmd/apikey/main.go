// apikey provisions and revokes read-only API credentials locally.
// Run on the same machine as the Site Observatory SQLite database.
package main

import (
 "flag"
 "fmt"
 "log"
 "os"
 "path/filepath"

 "example.com/site-observatory/internal/storage"
)
func main(){
 data:=flag.String("data","./data","directory containing observatory.db")
 name:=flag.String("name","","unique human-readable key name")
 revoke:=flag.Bool("revoke",false,"revoke existing key by name")
 flag.Parse()
 if *name==""{log.Fatal("specify -name")}
 dbPath:=filepath.Join(*data,"observatory.db")
 if _,err:=os.Stat(dbPath);err!=nil{log.Fatalf("existing Site Observatory database not found: %v",err)}
 db,err:=storage.Open(dbPath)
 if err!=nil{log.Fatal(err)}
 defer db.Close()
 if *revoke {
  if err=db.RevokeAPIKey(*name);err!=nil{log.Fatal(err)}
  fmt.Println("API key revoked:",*name)
  return
 }
 token,err:=db.CreateAPIKey(*name)
 if err!=nil{log.Fatal(err)}
 fmt.Fprintln(os.Stdout,"New read-only API key (shown once):")
 fmt.Fprintln(os.Stdout,token)
 fmt.Fprintln(os.Stdout,"Keep this token secret; only its hash is stored.")
}
