package protocol

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const Version = "commonbeam/v1"
const FakeModel = "commonbeam-simulated"
const FakeProfile = "simulation-v1"
const FakeText = "Commonbeam simulation. No AI model was executed."

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
}

func (r *ChatRequest) Validate() error {
	if r.Model == "" || len(r.Messages) == 0 || len(r.Messages) > 64 {
		return errors.New("model and 1-64 text messages are required")
	}
	if r.MaxTokens == 0 {
		r.MaxTokens = 512
	}
	if r.MaxTokens < 1 || r.MaxTokens > 512 {
		return errors.New("max_tokens must be between 1 and 512")
	}
	if r.Temperature != nil && *r.Temperature != 0 {
		return errors.New("only temperature 0 is supported in the test profile")
	}
	if r.Temperature == nil {
		zero := 0.0
		r.Temperature = &zero
	}
	for _, m := range r.Messages {
		if m.Role != "system" && m.Role != "user" && m.Role != "assistant" {
			return errors.New("unsupported message role")
		}
		if m.Content == "" {
			return errors.New("empty text messages are not supported")
		}
	}
	if r.Budget() > 4096 {
		return errors.New("conservative context budget exceeds 4096 units")
	}
	return nil
}

// Byte-based admission is deliberately conservative, not a model tokenizer.
func (r ChatRequest) InputBudget() int {
	n := 32
	for _, m := range r.Messages {
		n += len(m.Content) + 32
	}
	return n
}

func (r ChatRequest) Budget() int { return r.InputBudget() + r.MaxTokens }

func (r ChatRequest) Hash() string {
	r.Stream = false
	b, _ := json.Marshal(r)
	return Hash(b)
}

func Hash(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func RequestCommitment(salt string, request ChatRequest) string {
	if salt == "" {
		return request.Hash()
	}
	return Hash([]byte("commonbeam:request:v1\n" + salt + "\n" + request.Hash()))
}

func OutputCommitment(salt, output string) string {
	if salt == "" {
		return Hash([]byte(output))
	}
	return Hash([]byte("commonbeam:output:v1\n" + salt + "\n" + output))
}

func RandomID() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

type Manifest struct {
	Version   string `json:"version"`
	Network   string `json:"network"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	ModelHash string `json:"model_hash"`
	Profile   string `json:"profile"`
	Backend   string `json:"backend"`
	Capacity  int64  `json:"capacity"`
	Slots     int    `json:"slots"`
	Expires   int64  `json:"expires"`
	PublicKey string `json:"public_key"`
}

type SignedManifest struct {
	Manifest  Manifest `json:"manifest"`
	Signature string   `json:"signature"`
}

func PublicKey(key ed25519.PrivateKey) string {
	return base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
}

func ProviderID(public string) string { return Hash([]byte(public))[:32] }

func sign(domain string, payload any, key ed25519.PrivateKey) string {
	b, _ := json.Marshal(payload)
	return base64.StdEncoding.EncodeToString(ed25519.Sign(key, append([]byte(domain+"\n"), b...)))
}

func verify(domain string, payload any, public, sig string) bool {
	key, err := base64.StdEncoding.DecodeString(public)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return false
	}
	s, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return false
	}
	b, _ := json.Marshal(payload)
	return ed25519.Verify(key, append([]byte(domain+"\n"), b...), s)
}

func SignManifest(m Manifest, key ed25519.PrivateKey) SignedManifest {
	return SignedManifest{m, sign("commonbeam:manifest:v1", m, key)}
}

func (s SignedManifest) Validate(network string, now time.Time) error {
	m := s.Manifest
	if m.Version != Version || m.Network != network || m.Provider != ProviderID(m.PublicKey) {
		return errors.New("invalid manifest identity or network")
	}
	if m.Capacity < 1 || m.Capacity > 1_000_000_000 || m.Slots < 1 || m.Slots > 16 {
		return errors.New("capacity or slots outside test policy")
	}
	if m.Expires <= now.Unix() || m.Expires > now.Add(24*time.Hour).Unix() {
		return errors.New("manifest must expire within the next 24 hours")
	}
	if m.Model == "" || m.ModelHash == "" || m.Profile == "" {
		return errors.New("model identity and profile are required")
	}
	switch m.Backend {
	case "fake":
		if m.Model != FakeModel || m.Profile != FakeProfile || m.ModelHash != Hash([]byte(FakeText)) {
			return errors.New("invalid simulation profile")
		}
	case "vllm", "llamacpp":
		if m.Profile != "self-attested:"+m.Backend {
			return errors.New("real backend must declare self-attested verification")
		}
		hash, err := hex.DecodeString(m.ModelHash)
		if err != nil || len(hash) != 32 {
			return errors.New("real backend requires a SHA-256 model content hash")
		}
	default:
		return errors.New("unsupported backend")
	}
	if !verify("commonbeam:manifest:v1", m, m.PublicKey, s.Signature) {
		return errors.New("invalid manifest signature")
	}
	return nil
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type Receipt struct {
	Version     string `json:"version"`
	Network     string `json:"network"`
	Job         string `json:"job"`
	RequestHash string `json:"request_hash"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	Profile     string `json:"profile"`
	OutputHash  string `json:"output_hash"`
	Usage       Usage  `json:"usage"`
	Simulation  bool   `json:"simulation"`
}

type SignedReceipt struct {
	Receipt   Receipt `json:"receipt"`
	Signature string  `json:"signature"`
}

func SignReceipt(r Receipt, key ed25519.PrivateKey) SignedReceipt {
	return SignedReceipt{r, sign("commonbeam:receipt:v1", r, key)}
}

func (r SignedReceipt) Verify(public string) bool {
	return r.Receipt.Version == Version &&
		verify("commonbeam:receipt:v1", r.Receipt, public, r.Signature)
}

type Job struct {
	ID                string         `json:"id"`
	State             string         `json:"state"`
	Provider          string         `json:"provider"`
	Request           *ChatRequest   `json:"request,omitempty"`
	Hash              string         `json:"request_hash"`
	Output            string         `json:"output"`
	Reserved          int64          `json:"reserved"`
	Cost              int64          `json:"cost"`
	Deadline          int64          `json:"deadline"`
	Receipt           *SignedReceipt `json:"receipt,omitempty"`
	Audit             bool           `json:"audit_selected"`
	EvidenceAvailable bool           `json:"evidence_available"`
	Salt              string         `json:"commitment_salt,omitempty"`
}

func Terminal(state string) bool {
	return state == "complete" || state == "failed" || state == "cancelled" || state == "expired"
}

type Descriptor struct {
	Version   string `json:"version"`
	Network   string `json:"network"`
	Authority string `json:"authority"`
	Router    string `json:"router"`
	PublicKey string `json:"public_key"`
	Policy    string `json:"policy"`
}

type SignedDescriptor struct {
	Descriptor Descriptor `json:"descriptor"`
	Signature  string     `json:"signature"`
}

func SignDescriptor(d Descriptor, key ed25519.PrivateKey) SignedDescriptor {
	return SignedDescriptor{d, sign("commonbeam:descriptor:v1", d, key)}
}

func (d SignedDescriptor) Verify(fingerprint string) error {
	if d.Descriptor.Version != Version || Hash([]byte(d.Descriptor.PublicKey)) != fingerprint {
		return errors.New("descriptor version or pinned fingerprint mismatch")
	}
	if !verify("commonbeam:descriptor:v1", d.Descriptor, d.Descriptor.PublicKey, d.Signature) {
		return fmt.Errorf("invalid network descriptor signature")
	}
	return nil
}
