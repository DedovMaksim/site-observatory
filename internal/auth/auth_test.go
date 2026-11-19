package auth

import "testing"

func TestPassword(t *testing.T) {
	h, e := HashPassword("hello-world-password")
	if e != nil {
		t.Fatal(e)
	}
	if !VerifyPassword(h, "hello-world-password") {
		t.Fatal("should verify")
	}
	if VerifyPassword(h, "wrong-password") {
		t.Fatal("should reject")
	}
}
func TestSession(t *testing.T) {
	m, e := New(t.TempDir(), false)
	if e != nil {
		t.Fatal(e)
	}
	token := m.Session()
	if !m.Valid(token) {
		t.Fatal("token invalid")
	}
	if !m.ValidCSRF(token, m.CSRF(token)) {
		t.Fatal("csrf invalid")
	}
	if m.ValidCSRF(token, "wrong") {
		t.Fatal("csrf should reject")
	}
}
