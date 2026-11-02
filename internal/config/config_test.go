package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitPreservesIdentityAndJoinPin(t *testing.T) {
	dir := t.TempDir()
	fp, err := Init(dir, "http://127.0.0.1:8081", "http://127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	d, err := Descriptor(dir)
	if err != nil || d.Verify(fp) != nil {
		t.Fatal("initialized descriptor failed verification")
	}
	before, _ := os.ReadFile(filepath.Join(dir, "secrets.json"))
	if _, err := Init(dir, "http://127.0.0.1:9081", "http://127.0.0.1:9080"); err == nil {
		t.Fatal("init silently overwrote existing identity")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "secrets.json"))
	if string(before) != string(after) {
		t.Fatal("credentials changed after rejected init")
	}
	d.Descriptor.Authority = "http://127.0.0.1:1"
	if d.Verify(fp) == nil {
		t.Fatal("tampered descriptor accepted")
	}
}
