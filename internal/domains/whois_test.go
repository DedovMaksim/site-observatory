package domains

import (
 "context"
 "errors"
 "io"
 "net"
 "strings"
 "testing"
 "time"
)

func TestWHOISPaidTillFormats(t *testing.T) {
 cases:=[]struct{input,want string}{
  {"domain: MASALADABBA.RU\npaid-till: 2027-02-08T21:00:00Z\nfree-date: 2027-03-12","2027-02-08"},
  {"paid-till: 2027-05-12 12:30:00+0300\n","2027-05-12"},
  {"PAID-TILL: 2027-09-21\r\n","2027-09-21"},
 }
 for _,tc:=range cases{
  dt,err:=ParseWHOISPaidTill(tc.input)
  if err!=nil||dt.Format("2006-01-02")!=tc.want{t.Fatalf("parsed %s: %v, %v",tc.input,dt,err)}
 }
 if _,err:=ParseWHOISPaidTill("free-date: 2027-02-11\n");err==nil{t.Fatal("must never substitute free-date")}
}
func TestWHOISAllowedZones(t *testing.T){
 for _,domain:=range []string{"masaladabba.ru","xn--e1afmkfd.xn--p1ai"}{
  if err:=validateWHOISDomain(domain);err!=nil{t.Fatalf("%q rejected: %v",domain,err)}
 }
 for _,domain:=range []string{"example.com","a.ru\r\nexample.com","EXAMPLE.RU","a.ru:43","localhost","foo.local"}{
  if err:=validateWHOISDomain(domain);err==nil{t.Fatalf("%q incorrectly accepted",domain)}
 }
}
func TestWHOISLookupWithFixedServerAndBoundedReply(t *testing.T){
 clientConn,serverConn:=net.Pipe()
 defer serverConn.Close()
 observed:=make(chan string,1)
 go func(){
  defer serverConn.Close()
  incoming:=make([]byte,256)
  n,_:=serverConn.Read(incoming)
  observed<-string(incoming[:n])
  io.WriteString(serverConn,"domain: example.ru\npaid-till: 2027-04-08T12:00:00Z\n")
 }()
 w:=&WhoisClient{Dial:func(ctx context.Context,network,address string)(net.Conn,error){
  if network!="tcp"||address!=whoisAddress{t.Errorf("unexpected address %s %s",network,address)}
  return clientConn,nil
 }}
 result,err:=w.Lookup(context.Background(),"example.ru")
 if err!=nil{t.Fatal(err)}
 if result.Source!="WHOIS (TCI paid-till)"||result.Expires.Format("2006-01-02")!="2027-04-08"{t.Fatalf("unexpected result %+v",result)}
 select{case got:=<-observed:if got!="example.ru\r\n"{t.Errorf("query %q",got)}
 case <-time.After(time.Second):t.Fatal("no query captured")}
}
func TestWHOISLookupConnectionFailure(t *testing.T){
 w:=&WhoisClient{Dial:func(context.Context,string,string)(net.Conn,error){return nil,errors.New("port blocked")}}
 _,err:=w.Lookup(context.Background(),"example.ru")
 if err==nil||!strings.Contains(err.Error(),"port blocked"){t.Fatalf("expected clear network failure, got %v",err)}
}
