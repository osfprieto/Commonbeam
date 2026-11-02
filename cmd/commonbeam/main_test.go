package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/osfprieto/Commonbeam/internal/config"
)

func TestConsumerJoinDoesNotCopyOperatorPrivileges(t *testing.T) {
	owner, consumer := filepath.Join(t.TempDir(), "owner"), filepath.Join(t.TempDir(), "consumer")
	fp, err := config.Init(owner, "http://127.0.0.1:8081", "http://127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), []string{"join", "--state", consumer,
		"--descriptor", filepath.Join(owner, "network.json"), "--fingerprint", fp}, &out); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"COMMONBEAM_ADMIN_TOKEN", "COMMONBEAM_ROUTER_TOKEN", "COMMONBEAM_ENROLL_TOKEN", "COMMONBEAM_SPONSOR_TOKEN"} {
		t.Setenv(name, "")
	}
	if err := run(context.Background(), []string{"sponsor", "--state", consumer}, &out); err == nil {
		t.Fatal("joined consumer acquired sponsor privilege")
	}
}

func TestSessionDoesNotDiscardExistingAllowance(t *testing.T) {
	dir := t.TempDir()
	if _, err := config.Init(dir, "http://127.0.0.1:1", "http://127.0.0.1:1"); err != nil {
		t.Fatal(err)
	}
	if err := config.Write(filepath.Join(dir, "session.json"), session{Token: "saved"}, true); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), []string{"session", "--state", dir}, &out); err != nil {
		t.Fatal("existing allowance required a new network grant:", err)
	}
	var saved session
	config.Read(filepath.Join(dir, "session.json"), &saved)
	if saved.Token != "saved" {
		t.Fatal("session command discarded the existing allowance")
	}
	if err := run(context.Background(), []string{"contribute", "--state", dir}, &out); err == nil ||
		!strings.Contains(err.Error(), "--new-session") {
		t.Fatal("contribution command did not protect the selected account")
	}
}
