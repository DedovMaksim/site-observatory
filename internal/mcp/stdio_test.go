package mcp

import (
 "bytes"
 "encoding/json"
 "path/filepath"
 "strings"
 "testing"

 "example.com/site-observatory/internal/observatory"
 "example.com/site-observatory/internal/storage"
)
func TestMCPStdioLifecycleAndReadOnlyTools(t *testing.T){
 db,err:=storage.Open(filepath.Join(t.TempDir(),"mcp.db"));if err!=nil{t.Fatal(err)}
 defer db.Close()
 id,err:=db.CreateMonitor("Demo","https://example.com",60,10,true);if err!=nil{t.Fatal(err)}
 if err=db.SyncMonitorDomain(id,"https://example.com");err!=nil{t.Fatal(err)}
 srv:=Server{Service:observatory.New(db)}
 requests:=[]string{
  `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
  `{"jsonrpc":"2.0","method":"notifications/initialized"}`,
  `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
  `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_summary","arguments":{}}}`,
  `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get_monitor_status","arguments":{"monitor_id":1}}}`,
  `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"get_domain_expiry","arguments":{}}}`,
  `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"delete_monitor","arguments":{"monitor_id":1}}}`,
  `{"jsonrpc":"2.0","id":7,"method":"bad_method"}`,
 }
 var output bytes.Buffer
 if err=srv.Serve(strings.NewReader(strings.Join(requests,"\n")+"\n"),&output);err!=nil{t.Fatal(err)}
 lines:=strings.Split(strings.TrimSpace(output.String()),"\n")
 if len(lines)!=7{t.Fatalf("expected 7 JSON-RPC responses, got %d: %s",len(lines),output.String())}
 var parsed []map[string]any
 for _,line:=range lines{
  var m map[string]any
  if err=json.Unmarshal([]byte(line),&m);err!=nil{t.Fatalf("non-JSON stdout: %v",err)}
  parsed=append(parsed,m)
 }
 info:=parsed[0]["result"].(map[string]any)
 if info["protocolVersion"]!="2025-11-25"{t.Fatalf("protocol negotiation: %+v",info)}
 listed:=parsed[1]["result"].(map[string]any)["tools"].([]any)
 if len(listed)!=6{t.Fatalf("expected six tools, got %d",len(listed))}
 for _,v:=range listed{
  info:=v.(map[string]any)
  annotations:=info["annotations"].(map[string]any)
  if annotations["readOnlyHint"]!=true{t.Fatal("non-read-only tool listed")}
 }
 for _,i:=range []int{2,3,4}{
  result:=parsed[i]["result"].(map[string]any)
  if result["isError"]!=false{t.Fatalf("expected successful tool call: %+v",result)}
 }
 if parsed[5]["result"].(map[string]any)["isError"]!=true{t.Fatal("unknown tool wasn't rejected")}
 if parsed[6]["error"].(map[string]any)["code"].(float64)!=-32601{t.Fatal("unknown RPC method wasn't rejected")}
 if _,err=db.Monitor(id);err!=nil{t.Fatal("read-only MCP changed monitor")}
}
func TestMCPRejectBadParams(t *testing.T){
 srv:=Server{}
 for _,raw:=range []string{
  `{"name":"get_monitor_status","arguments":{}}`,
  `{"name":"list_incidents","arguments":{"limit":500}}`,
  `{"name":"get_summary","arguments":{"secret":"x"}}`,
  `{"name":"get_seo_issues","arguments":{"monitor_id":"1"}}`,
 }{
  if _,err:=call(srv.Service,json.RawMessage(raw));err==nil{t.Fatalf("invalid parameters accepted: %s",raw)}
 }
}
