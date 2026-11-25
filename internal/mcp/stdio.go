// Package mcp implements a local read-only MCP stdio server.
// It supports the widely deployed 2025-11-25 initialize lifecycle and
// newline-delimited JSON-RPC messages. No HTTP listener is started.
package mcp

import (
 "bufio"
 "bytes"
 "database/sql"
 "encoding/json"
 "errors"
 "fmt"
 "io"

 "example.com/site-observatory/internal/observatory"
)

type Service interface {
 Monitors()([]observatory.Monitor,error)
 Summary()(observatory.Summary,error)
 MonitorDetail(int64)(observatory.MonitorDetail,error)
 Incidents(int64,int)([]observatory.Incident,error)
 SEOIssues(int64,int)([]observatory.SEOIssue,error)
 Domains()([]observatory.Domain,error)
}
type Server struct{Service Service}
type request struct{
 JSONRPC string `json:"jsonrpc"`
 ID json.RawMessage `json:"id"`
 Method string `json:"method"`
 Params json.RawMessage `json:"params"`
}
type rpcError struct{Code int `json:"code"`;Message string `json:"message"`}
type response struct{JSONRPC string `json:"jsonrpc"`;ID json.RawMessage `json:"id"`;Result any `json:"result,omitempty"`;Error *rpcError `json:"error,omitempty"`}
type tool struct{
 Name string `json:"name"`
 Description string `json:"description"`
 InputSchema any `json:"inputSchema"`
 Annotations map[string]any `json:"annotations"`
}
func schema(properties map[string]any,required ...string)any{
 if required==nil {required=[]string{}}
 return map[string]any{"type":"object","properties":properties,"required":required,"additionalProperties":false}
}
func idProperty()any{return map[string]any{"type":"integer","minimum":1,"description":"Existing monitor ID"}}
func limitProperty()any{return map[string]any{"type":"integer","minimum":1,"maximum":100,"default":25}}
var tools=[]tool{
 {"get_summary","Overall monitor health and open incident count",schema(map[string]any{}),map[string]any{"readOnlyHint":true}},
 {"list_monitors","List monitored websites and current status",schema(map[string]any{}),map[string]any{"readOnlyHint":true}},
 {"get_monitor_status","Get checks, incidents, SSL, DNS, domain, sitemap, robots and SEO overview for one monitor",schema(map[string]any{"monitor_id":idProperty()},"monitor_id"),map[string]any{"readOnlyHint":true}},
 {"list_incidents","List recent incidents, optionally for one monitor",schema(map[string]any{"monitor_id":idProperty(),"limit":limitProperty()}),map[string]any{"readOnlyHint":true}},
 {"get_seo_issues","List current HTTP and metadata issues for pages from a monitor sitemap",schema(map[string]any{"monitor_id":idProperty(),"limit":limitProperty()},"monitor_id"),map[string]any{"readOnlyHint":true}},
 {"get_domain_expiry","List registered domains and known expiry dates",schema(map[string]any{}),map[string]any{"readOnlyHint":true}},
}
func rpcFailure(id json.RawMessage,code int,msg string)response{return response{JSONRPC:"2.0",ID:id,Error:&rpcError{Code:code,Message:msg}}}
func argNumber(m map[string]json.RawMessage,key string,defaultValue int64)(int64,error){
 raw,ok:=m[key];if !ok{return defaultValue,nil}
 var v int64
 if err:=json.Unmarshal(raw,&v);err!=nil{return 0,fmt.Errorf("%s must be an integer",key)}
 return v,nil
}
func call(s Service,raw json.RawMessage)(any,error){
 var p struct{Name string `json:"name"`;Arguments map[string]json.RawMessage `json:"arguments"`}
 if err:=json.Unmarshal(raw,&p);err!=nil{return nil,errors.New("invalid tool arguments")}
 if p.Arguments==nil{p.Arguments=map[string]json.RawMessage{}}
 var allowed map[string]bool
 switch p.Name{
 case "get_summary","list_monitors","get_domain_expiry": allowed=map[string]bool{}
 case "get_monitor_status","get_seo_issues":allowed=map[string]bool{"monitor_id":true,"limit":p.Name=="get_seo_issues"}
 case "list_incidents":allowed=map[string]bool{"monitor_id":true,"limit":true}
 default:return nil,fmt.Errorf("unknown tool %q",p.Name)
 }
 for k:=range p.Arguments{if !allowed[k]{return nil,fmt.Errorf("unknown argument %q",k)}}
 id,err:=argNumber(p.Arguments,"monitor_id",0);if err!=nil{return nil,err}
 if (p.Name=="get_monitor_status"||p.Name=="get_seo_issues") && id<=0{return nil,errors.New("monitor_id must be positive")}
 if id<0{return nil,errors.New("monitor_id must not be negative")}
 n,err:=argNumber(p.Arguments,"limit",25);if err!=nil{return nil,err}
 if n<1||n>100{return nil,errors.New("limit must be 1..100")}
 switch p.Name{
 case "get_summary":return s.Summary()
 case "list_monitors":return s.Monitors()
 case "get_monitor_status":return s.MonitorDetail(id)
 case "list_incidents":return s.Incidents(id,int(n))
 case "get_seo_issues":return s.SEOIssues(id,int(n))
 case "get_domain_expiry":return s.Domains()
 }
 return nil,errors.New("unknown tool")
}
func (s *Server) handle(line []byte)*response{
 var req request
 if err:=json.Unmarshal(line,&req);err!=nil{r:=rpcFailure(json.RawMessage("null"),-32700,"parse error");return &r}
 if req.JSONRPC!="2.0"||req.Method==""{r:=rpcFailure(json.RawMessage("null"),-32600,"invalid request");return &r}
 // JSON-RPC notifications have no ID and receive no response.
 if len(req.ID)==0{return nil}
 if string(req.ID)=="null"{r:=rpcFailure(req.ID,-32600,"invalid request id");return &r}
 result:=any(nil)
 switch req.Method{
 case "initialize":
  result=map[string]any{
   "protocolVersion":"2025-11-25",
   "capabilities":map[string]any{"tools":map[string]any{"listChanged":false}},
   "serverInfo":map[string]any{"name":"site-observatory","version":"0.8.0"},
  }
 case "ping":result=map[string]any{}
 case "tools/list":result=map[string]any{"tools":tools}
 case "tools/call":
  value,err:=call(s.Service,req.Params)
  if err!=nil{
   if errors.Is(err,sql.ErrNoRows){err=errors.New("monitor not found")}
   result=map[string]any{"isError":true,"content":[]any{map[string]any{"type":"text","text":err.Error()}}}
  }else{
   b,e:=json.Marshal(value)
   if e!=nil{result=map[string]any{"isError":true,"content":[]any{map[string]any{"type":"text","text":"serialization failed"}}}
   }else{result=map[string]any{"content":[]any{map[string]any{"type":"text","text":string(b)}},"structuredContent":map[string]any{"data":value},"isError":false}}
  }
 default:
  r:=rpcFailure(req.ID,-32601,"method not found");return &r
 }
 return &response{JSONRPC:"2.0",ID:req.ID,Result:result}
}
func(s *Server) Serve(in io.Reader,out io.Writer)error{
 scanner:=bufio.NewScanner(in)
 scanner.Buffer(make([]byte,4096),1<<20)
 writer:=bufio.NewWriter(out)
 for scanner.Scan(){
  line:=bytes.TrimSpace(scanner.Bytes())
  if len(line)==0{continue}
  resp:=s.handle(line)
  if resp==nil{continue}
  encoded,err:=json.Marshal(resp)
  if err!=nil{return err}
  if _,err=writer.Write(encoded);err!=nil{return err}
  if err=writer.WriteByte('\n');err!=nil{return err}
  if err=writer.Flush();err!=nil{return err}
 }
 return scanner.Err()
}
