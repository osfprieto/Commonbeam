package config

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/osfprieto/Commonbeam/internal/protocol"
)

type Secrets struct {
	Admin    string `json:"admin"`
	Router   string `json:"router"`
	Enroll   string `json:"enroll"`
	Sponsor  string `json:"sponsor"`
	Evidence string `json:"evidence"`
}

func Write(path string, value any, exclusive bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if exclusive {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = f.Write(append(b, '\n'))
		closeErr := f.Close()
		if err != nil {
			return err
		}
		return closeErr
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".commonbeam-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func Read(path string, out any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return errors.New("configuration file missing or unreadable: " + filepath.Base(path))
	}
	return json.Unmarshal(b, out)
}

func Init(dir, authority, router string) (string, error) {
	if _, err := os.Stat(filepath.Join(dir, "network.json")); err == nil {
		return "", errors.New("network already initialized; refusing to overwrite identity")
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	evidence := make([]byte, 32)
	if _, err := rand.Read(evidence); err != nil {
		return "", err
	}
	d := protocol.SignDescriptor(protocol.Descriptor{
		Version: protocol.Version, Network: protocol.RandomID(), Authority: authority, Router: router,
		PublicKey: protocol.PublicKey(key), Policy: "test-only; single-authority; router/provider plaintext",
	}, key)
	secrets := Secrets{protocol.RandomID(), protocol.RandomID(), protocol.RandomID(), protocol.RandomID(), base64.StdEncoding.EncodeToString(evidence)}
	fingerprint := protocol.Hash([]byte(d.Descriptor.PublicKey))
	for _, file := range []struct {
		name  string
		value any
	}{{"secrets.json", secrets}, {"trust.json", map[string]string{"fingerprint": fingerprint}}, {"network.json", d}} {
		if err := Write(filepath.Join(dir, file.name), file.value, true); err != nil {
			return "", err
		}
	}
	return fingerprint, nil
}

func Descriptor(dir string) (protocol.SignedDescriptor, error) {
	var d protocol.SignedDescriptor
	var trust struct {
		Fingerprint string `json:"fingerprint"`
	}
	if err := Read(filepath.Join(dir, "network.json"), &d); err != nil {
		return d, err
	}
	if err := Read(filepath.Join(dir, "trust.json"), &trust); err != nil {
		return d, err
	}
	return d, d.Verify(trust.Fingerprint)
}

func LoadSecrets(dir string) (Secrets, error) {
	var s Secrets
	err := Read(filepath.Join(dir, "secrets.json"), &s)
	if err != nil {
		s = Secrets{Admin: os.Getenv("COMMONBEAM_ADMIN_TOKEN"), Router: os.Getenv("COMMONBEAM_ROUTER_TOKEN"),
			Enroll: os.Getenv("COMMONBEAM_ENROLL_TOKEN"), Sponsor: os.Getenv("COMMONBEAM_SPONSOR_TOKEN"),
			Evidence: os.Getenv("COMMONBEAM_EVIDENCE_KEY")}
		if s.Admin != "" || s.Router != "" || s.Enroll != "" || s.Sponsor != "" {
			err = nil
		}
	}
	return s, err
}

func ProviderKey(path string) (ed25519.PrivateKey, error) {
	var saved struct {
		Private string `json:"private"`
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		saved.Private = base64.StdEncoding.EncodeToString(key)
		if err := Write(path, saved, true); err != nil {
			return nil, err
		}
		return key, nil
	}
	if err := Read(path, &saved); err != nil {
		return nil, err
	}
	key, err := base64.StdEncoding.DecodeString(saved.Private)
	if err != nil || len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid provider key file")
	}
	return ed25519.PrivateKey(key), nil
}
