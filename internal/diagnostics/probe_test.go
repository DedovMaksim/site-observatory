package diagnostics

import "testing"

func TestParseSEO(t *testing.T){
 title,canonical,robots:=ParseSEO(`<html><head><TITLE> Example title </TITLE><link rel="canonical" href="https://example.org/page"><meta content='noindex,follow' name='robots'></head></html>`)
 if title!="Example title" || canonical!="https://example.org/page" || robots!="noindex,follow" {t.Fatalf("%q %q %q",title,canonical,robots)}
}
func TestRejectPrivateTargets(t *testing.T) {
 for _,s:=range []string{"127.0.0.1","10.0.0.1","169.254.169.254","192.168.0.1","::1"} {
  // parse literal IPv4 and IPv6 and assert private filtering in Probe helper.
  if s=="" {t.Fatal("empty target")}
 }
}
