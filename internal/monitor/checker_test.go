package monitor

import (
	"errors"
	"net"
	"testing"
)

func TestValidURLs(t *testing.T) {
	c := Checker{}
	for _, u := range []string{"file:///etc/passwd", "http://localhost/", "http://admin:secret@example.org/", "https://foo.local/"} {
		if c.ValidateURL(u) == nil {
			t.Fatalf("should block %s", u)
		}
	}
	if err := c.ValidateURL("https://example.org/"); err != nil {
		t.Fatal(err)
	}
}
func TestBlockedAddresses(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.0.0.3", "192.168.1.1", "169.254.169.254", "::1", "fc00::1", "0.0.0.0"} {
		if !restricted(net.ParseIP(s)) {
			t.Fatalf("expected blocked %s", s)
		}
	}
	if restricted(net.ParseIP("8.8.8.8")) {
		t.Fatal("public IP rejected")
	}
	if !errors.Is(ErrBlocked, ErrBlocked) {
		t.Fatal("sentinel broken")
	}
}
