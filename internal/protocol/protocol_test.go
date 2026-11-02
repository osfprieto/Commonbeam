package protocol

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"
)

func TestSignedManifestBinding(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Unix(1700000000, 0)
	m := Manifest{Version: Version, Network: "n", Provider: ProviderID(PublicKey(key)),
		Model: FakeModel, ModelHash: Hash([]byte(FakeText)), Profile: FakeProfile,
		Backend: "fake", Capacity: 10000, Slots: 1, Expires: now.Add(time.Hour).Unix(), PublicKey: PublicKey(key)}
	s := SignManifest(m, key)
	if err := s.Validate("n", now); err != nil {
		t.Fatal(err)
	}
	if s.Signature != SignManifest(m, key).Signature {
		t.Fatal("manifest serialization is not deterministic")
	}
	for _, mutate := range []func(*SignedManifest){
		func(s *SignedManifest) { s.Manifest.Capacity++ },
		func(s *SignedManifest) { s.Manifest.Network = "other" },
		func(s *SignedManifest) { s.Manifest.Provider = "other" },
		func(s *SignedManifest) { s.Manifest.Profile = "real" },
	} {
		bad := s
		mutate(&bad)
		if bad.Validate("n", now) == nil {
			t.Fatal("tampered manifest accepted")
		}
	}
	if s.Validate("n", now.Add(2*time.Hour)) == nil {
		t.Fatal("expired manifest accepted")
	}
}

func TestRequestBoundsAndIdempotencyHash(t *testing.T) {
	r := ChatRequest{Model: FakeModel, Messages: []Message{{"user", "hello"}}}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	hash := r.Hash()
	r.Stream = true
	if hash != r.Hash() {
		t.Fatal("stream transport changed semantic request identity")
	}
	r.MaxTokens = 513
	if r.Validate() == nil {
		t.Fatal("unbounded output accepted")
	}
	r.MaxTokens = 1
	r.Messages[0].Role = "tool"
	if r.Validate() == nil {
		t.Fatal("unsupported role accepted")
	}
}

func TestDescriptorPinAndReceiptDomain(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	d := SignDescriptor(Descriptor{Version: Version, Network: "n", PublicKey: PublicKey(key)}, key)
	if err := d.Verify(Hash([]byte(PublicKey(key)))); err != nil {
		t.Fatal(err)
	}
	if d.Verify(Hash([]byte("attacker"))) == nil {
		t.Fatal("incorrect fingerprint accepted")
	}
	r := SignReceipt(Receipt{Version: Version, Network: "n", Job: "j"}, key)
	if !r.Verify(PublicKey(key)) {
		t.Fatal("valid receipt rejected")
	}
	r.Receipt.Network = "other"
	if r.Verify(PublicKey(key)) {
		t.Fatal("receipt network tampering accepted")
	}
}
