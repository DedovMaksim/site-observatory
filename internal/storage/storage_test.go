package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestIncidentLifecycle(t *testing.T) {
	db, e := Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	id, e := db.CreateMonitor("Test", "https://example.org", 15, 3, true)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 3; i++ {
		if e = db.Record(context.Background(), id, Check{CheckedAt: time.Now().UTC().Add(time.Duration(i) * time.Second).Format(time.RFC3339), Success: false, Error: "timeout"}); e != nil {
			t.Fatal(e)
		}
	}
	m, e := db.Monitor(id)
	if e != nil || m.Status != "DOWN" {
		t.Fatalf("expected DOWN: %+v %v", m, e)
	}
	for i := 0; i < 2; i++ {
		if e = db.Record(context.Background(), id, Check{CheckedAt: time.Now().UTC().Add(time.Duration(i+3) * time.Second).Format(time.RFC3339), Success: true, Code: 200}); e != nil {
			t.Fatal(e)
		}
	}
	m, e = db.Monitor(id)
	if e != nil || m.Status != "UP" {
		t.Fatalf("expected UP: %+v %v", m, e)
	}
	inc, e := db.Incidents(id, 10)
	if e != nil || len(inc) != 1 || inc[0].EndedAt == "" {
		t.Fatalf("expected closed incident: %+v %v", inc, e)
	}
}
