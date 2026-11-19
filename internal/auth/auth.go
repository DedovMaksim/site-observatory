package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// PBKDF2-HMAC-SHA256, 240k iterations, random 16-byte salt.
func derive(pass string, salt []byte, iterations int) []byte {
	block := make([]byte, len(salt)+4)
	copy(block, salt)
	binary.BigEndian.PutUint32(block[len(salt):], 1)
	mac := hmac.New(sha256.New, []byte(pass))
	mac.Write(block)
	u := mac.Sum(nil)
	result := append([]byte(nil), u...)
	for i := 1; i < iterations; i++ {
		mac = hmac.New(sha256.New, []byte(pass))
		mac.Write(u)
		u = mac.Sum(nil)
		for j := range result {
			result[j] ^= u[j]
		}
	}
	return result
}
func HashPassword(password string) (string, error) {
	if len(password) < 12 {
		return "", errors.New("ADMIN_PASSWORD must be at least 12 characters")
	}
	salt := make([]byte, 16)
	if _, e := rand.Read(salt); e != nil {
		return "", e
	}
	h := derive(password, salt, 240000)
	return fmt.Sprintf("pbkdf2_sha256$240000$%s$%s", base64.RawURLEncoding.EncodeToString(salt), base64.RawURLEncoding.EncodeToString(h)), nil
}
func VerifyPassword(encoded, password string) bool {
	p := strings.Split(encoded, "$")
	if len(p) != 4 || p[0] != "pbkdf2_sha256" {
		return false
	}
	n, e := strconv.Atoi(p[1])
	if e != nil || n < 10000 || n > 1000000 {
		return false
	}
	s, e := base64.RawURLEncoding.DecodeString(p[2])
	if e != nil {
		return false
	}
	want, e := base64.RawURLEncoding.DecodeString(p[3])
	if e != nil {
		return false
	}
	got := derive(password, s, n)
	return subtle.ConstantTimeCompare(want, got) == 1
}

type Manager struct {
	secret []byte
	Secure bool
}

func New(dataDir string, secure bool) (*Manager, error) {
	path := filepath.Join(dataDir, "session.key")
	s, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		s = make([]byte, 32)
		if _, e = rand.Read(s); e != nil {
			return nil, e
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return nil, err
		}
		_, e = f.Write(s)
		f.Close()
		if e != nil {
			return nil, e
		}
	} else if e != nil {
		return nil, e
	}
	if len(s) != 32 {
		return nil, errors.New("invalid session.key")
	}
	return &Manager{s, secure}, nil
}
func (m *Manager) sign(s string) string {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(s))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (m *Manager) Session() string {
	nonce := make([]byte, 16)
	rand.Read(nonce)
	data := strconv.FormatInt(time.Now().Add(24*time.Hour).Unix(), 10) + "." + base64.RawURLEncoding.EncodeToString(nonce)
	return data + "." + m.sign(data)
}
func (m *Manager) Valid(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	exp, e := strconv.ParseInt(parts[0], 10, 64)
	if e != nil || time.Now().Unix() > exp {
		return false
	}
	data := parts[0] + "." + parts[1]
	return hmac.Equal([]byte(m.sign(data)), []byte(parts[2]))
}
func (m *Manager) CSRF(token string) string { return m.sign("csrf:" + token) }
func (m *Manager) ValidCSRF(token, received string) bool {
	return token != "" && hmac.Equal([]byte(m.CSRF(token)), []byte(received))
}
