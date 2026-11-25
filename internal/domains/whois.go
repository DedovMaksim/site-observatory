package domains

import (
 "bufio"
 "context"
 "errors"
 "fmt"
 "io"
 "net"
 "regexp"
 "strings"
 "time"
)

// The TCI registry publishes paid-till for .ru and .рф domains.
// Only a fixed registry host is used; incoming monitor URLs cannot choose
// a WHOIS destination or port.
const whoisAddress = "whois.tcinet.ru:43"
const maxWHOISResponse = 64 * 1024

var whoisName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

func SupportsTCIWhois(domain string) bool {
 d := strings.ToLower(domain)
 return strings.HasSuffix(d, ".ru") || strings.HasSuffix(d, ".xn--p1ai")
}

func validateWHOISDomain(domain string) error {
 if len(domain) < 4 || len(domain) > 253 || domain != strings.ToLower(domain) || !whoisName.MatchString(domain) {
  return errors.New("invalid WHOIS domain")
 }
 if !SupportsTCIWhois(domain) {
  return errors.New("WHOIS fallback is supported only for .ru and .рф")
 }
 return nil
}

// ParseWHOISPaidTill extracts only the paid-till attribute (never free-date).
// Whois records usually publish RFC3339 timestamps, but older records can
// contain dates without a timezone.
func ParseWHOISPaidTill(data string) (time.Time, error) {
 for _,line := range strings.Split(data, "\n") {
  s := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
  parts := strings.SplitN(s, ":", 2)
  if len(parts) != 2 || !strings.EqualFold(strings.TrimSpace(parts[0]), "paid-till") {
   continue
  }
  raw := strings.TrimSpace(parts[1])
  for _,layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05-0700", "2006-01-02 15:04:05Z0700", "2006-01-02"} {
   if t,err := time.Parse(layout,raw);err == nil {
    return t.UTC(),nil
   }
  }
  return time.Time{},fmt.Errorf("unrecognized WHOIS paid-till format")
 }
 return time.Time{},errors.New("WHOIS did not provide paid-till; enter expiry manually")
}

// WhoisClient supports dependency injection for unit tests; production uses
// a fixed whois.tcinet.ru:43 address and bounded response.
type WhoisClient struct {
 Dial func(ctx context.Context, network, address string) (net.Conn,error)
}

func (c *WhoisClient) Lookup(ctx context.Context,domain string) (LookupResult,error) {
 if err := validateWHOISDomain(domain);err != nil {return LookupResult{},err}
 dial := c.Dial
 if dial == nil {
  d:=&net.Dialer{Timeout:6*time.Second}
  dial=d.DialContext
 }
 ctx,cancel:=context.WithTimeout(ctx,10*time.Second)
 defer cancel()
 conn,err:=dial(ctx,"tcp",whoisAddress)
 if err!=nil{return LookupResult{},fmt.Errorf("WHOIS TCP connection failed: %w",err)}
 defer conn.Close()
 deadline:=time.Now().Add(9*time.Second)
 if d,ok:=ctx.Deadline();ok&&d.Before(deadline){deadline=d}
 if err:=conn.SetDeadline(deadline);err!=nil{return LookupResult{},err}
 if _,err=io.WriteString(conn,domain+"\r\n");err!=nil{return LookupResult{},fmt.Errorf("WHOIS query failed: %w",err)}
 // LimitReader returns at most max+1, so oversized replies are rejected.
 reader:=bufio.NewReader(io.LimitReader(conn,maxWHOISResponse+1))
 data,err:=io.ReadAll(reader)
 if err!=nil{return LookupResult{},fmt.Errorf("WHOIS response failed: %w",err)}
 if len(data)>maxWHOISResponse{return LookupResult{},errors.New("WHOIS response too large")}
 expiry,err:=ParseWHOISPaidTill(string(data))
 if err!=nil{return LookupResult{},err}
 return LookupResult{Expires:expiry,Source:"WHOIS (TCI paid-till)"},nil
}
