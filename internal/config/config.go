package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Addr, DataDir, PublicURL, AdminPassword string
	CookieSecure, AllowPrivateTargets       bool
}

func Load() Config {
	readDotEnv(".env")
	return Config{Addr: get("APP_ADDR", "127.0.0.1:8080"), DataDir: get("DATA_DIR", "./data"), PublicURL: get("APP_PUBLIC_URL", "http://localhost:8080"), AdminPassword: os.Getenv("ADMIN_PASSWORD"), CookieSecure: get("COOKIE_SECURE", "false") == "true", AllowPrivateTargets: get("ALLOW_PRIVATE_TARGETS", "false") == "true"}
}
func get(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func readDotEnv(path string) {
	f, e := os.Open(path)
	if e != nil {
		return
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		l := strings.TrimSpace(s.Text())
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		a := strings.SplitN(l, "=", 2)
		if len(a) != 2 {
			continue
		}
		k := strings.TrimSpace(a[0])
		v := strings.Trim(strings.TrimSpace(a[1]), "\"'")
		if _, ok := os.LookupEnv(k); !ok {
			os.Setenv(k, v)
		}
	}
}
func (c Config) Validate() error {
	if err := os.MkdirAll(c.DataDir, 0700); err != nil {
		return err
	}
	abs, err := filepath.Abs(c.DataDir)
	if err != nil {
		return err
	}
	if c.Addr == "" {
		return fmt.Errorf("APP_ADDR is required")
	}
	if c.CookieSecure && !strings.HasPrefix(c.PublicURL, "https://") {
		return fmt.Errorf("COOKIE_SECURE requires HTTPS APP_PUBLIC_URL")
	}
	_ = abs
	return nil
}
