package diagnostics

import (
 "net"
 "testing"
)

func TestParseSEO(t *testing.T) {
 title,canonical,robots:=ParseSEO(`<html><head><TITLE> Example title </TITLE><link rel="canonical" href="https://example.org/page"><meta content='noindex,follow' name='robots'></head></html>`)
 if title!="Example title" || canonical!="https://example.org/page" || robots!="noindex,follow" {t.Fatalf("%q %q %q",title,canonical,robots)}
}
func TestIPRestrictions(t *testing.T) {
 for _,s:=range []string{"127.0.0.1","10.0.0.1","169.254.169.254","192.168.0.1","::1","fd00::1","100.64.0.1","198.18.0.1"} {
  if public(net.ParseIP(s)) {t.Errorf("unsafe IP accepted: %s",s)}
 }
 for _,s:=range []string{"8.8.8.8","1.1.1.1","2606:4700:4700::1111"} {
  if !public(net.ParseIP(s)){t.Errorf("public IP rejected: %s",s)}
 }
}
func TestURLValidation(t *testing.T) {
 for _,s:=range []string{"file:///etc/passwd","http://user:password@example.com","http://","ftp://example.com"} {
  if _,err:=hostFrom(s);err==nil {t.Errorf("unsafe URL accepted: %s",s)}
 }
}
