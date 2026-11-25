// Site Observatory MCP — local read-only stdio transport.
// Stdout is reserved exclusively for JSON-RPC messages.
package main

import (
 "flag"
 "log"
 "os"
 "path/filepath"

 "example.com/site-observatory/internal/mcp"
 "example.com/site-observatory/internal/observatory"
 "example.com/site-observatory/internal/storage"
)
func main(){
 data:=flag.String("data","./data","Site Observatory data directory")
 flag.Parse()
 dbPath:=filepath.Join(*data,"observatory.db")
 if _,err:=os.Stat(dbPath);err!=nil{log.Fatalf("existing Site Observatory database not found: %v",err)}
 db,err:=storage.Open(dbPath)
 if err!=nil{log.Fatal(err)}
 defer db.Close()
 server:=mcp.Server{Service:observatory.New(db)}
 if err=server.Serve(os.Stdin,os.Stdout);err!=nil{log.Fatal(err)}
}
