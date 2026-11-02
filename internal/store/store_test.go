package store_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/osfprieto/Commonbeam/internal/protocol"
	"github.com/osfprieto/Commonbeam/internal/store"
	"github.com/osfprieto/Commonbeam/internal/testutil"
)

var ctx = context.Background()

func enroll(t *testing.T, db *store.Store, slots int) (protocol.Manifest, ed25519.PrivateKey, string) {
	t.Helper()
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	m := protocol.Manifest{Version: protocol.Version, Network: db.Network,
		Provider: protocol.ProviderID(protocol.PublicKey(key)), Model: protocol.FakeModel,
		ModelHash: protocol.Hash([]byte(protocol.FakeText)), Profile: protocol.FakeProfile,
		Backend: "fake", Capacity: 100000, Slots: slots, Expires: time.Now().Add(time.Hour).Unix(),
		PublicKey: protocol.PublicKey(key)}
	token, err := db.Register(ctx, protocol.SignManifest(m, key))
	if err != nil {
		t.Fatal(err)
	}
	return m, key, token
}

func grant(t *testing.T, db *store.Store) string {
	t.Helper()
	if err := db.Fund(ctx, "public", 4096); err != nil {
		t.Fatal(err)
	}
	token, err := db.Grant(ctx, "public", 4096)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func request() protocol.ChatRequest {
	return protocol.ChatRequest{Model: protocol.FakeModel, MaxTokens: 512,
		Messages: []protocol.Message{{Role: "user", Content: "private-prompt-never-plaintext"}}}
}

func TestConcurrentReservationsAcrossAuthorityInstances(t *testing.T) {
	db, dsn, key := testutil.Database(t)
	enroll(t, db, 2)
	second, err := store.Open(ctx, dsn, db.Network, key)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	var tokens []string
	for range 16 {
		tokens = append(tokens, grant(t, db))
	}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i, token := range tokens {
		wg.Add(1)
		go func(i int, token string) {
			defer wg.Done()
			target := db
			if i%2 == 0 {
				target = second
			}
			_, err := target.Reserve(ctx, token, protocol.RandomID(), request())
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, store.ErrCapacity) {
				t.Errorf("unexpected reservation error: %v", err)
			}
		}(i, token)
	}
	wg.Wait()
	if successes.Load() != 2 {
		t.Fatalf("capacity double-booked or lost: %d reservations", successes.Load())
	}
	var available, holds, balances, pools int64
	db.DB.QueryRow(ctx, "SELECT sum(available) FROM providers").Scan(&available)
	db.DB.QueryRow(ctx, "SELECT sum(reserved) FROM jobs").Scan(&holds)
	db.DB.QueryRow(ctx, "SELECT sum(balance) FROM accounts").Scan(&balances)
	db.DB.QueryRow(ctx, "SELECT sum(balance) FROM pools").Scan(&pools)
	if available+holds != 100000 || pools+balances+holds != 16*4096 {
		t.Fatalf("conservation failed: capacity=%d holds=%d balances=%d pools=%d", available, holds, balances, pools)
	}
	if err := db.Fund(ctx, "paid", 100000); !errors.Is(err, store.ErrCapacity) {
		t.Fatalf("unbacked test credits accepted: %v", err)
	}
}

func TestIdempotencyEncryptionAndRecovery(t *testing.T) {
	db, dsn, key := testutil.Database(t)
	enroll(t, db, 1)
	token := grant(t, db)
	idem := protocol.RandomID()
	var wg sync.WaitGroup
	ids := make(chan string, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			j, err := db.Reserve(ctx, token, idem, request())
			if err != nil {
				t.Error(err)
			} else {
				ids <- j.ID
			}
		}()
	}
	wg.Wait()
	close(ids)
	var id string
	for value := range ids {
		if id != "" && value != id {
			t.Fatal("idempotent request executed twice")
		}
		id = value
	}
	var raw []byte
	if err := db.DB.QueryRow(ctx, "SELECT request FROM jobs WHERE id=$1", id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("private-prompt")) {
		t.Fatal("prompt persisted in plaintext")
	}
	details, err := db.Job(ctx, id, token, "", false)
	if err != nil {
		t.Fatal(err)
	}
	canonical := request()
	if err := canonical.Validate(); err != nil {
		t.Fatal(err)
	}
	if details.Salt == "" || bytes.Contains(raw, []byte(details.Salt)) ||
		details.Hash == canonical.Hash() ||
		details.Hash != protocol.RequestCommitment(details.Salt, canonical) {
		t.Fatal("private salted request commitment was not preserved")
	}
	canonical.Stream = true
	replay, err := db.Reserve(ctx, token, idem, canonical)
	if err != nil || replay.Salt != details.Salt || replay.Hash != details.Hash {
		t.Fatalf("idempotent replay changed private commitments: %v", err)
	}
	if balance, err := db.Balance(ctx, token); err != nil || balance != 4096-details.Reserved {
		t.Fatalf("idempotent replay changed the reserved budget: %d %v", balance, err)
	}
	changed := request()
	changed.Messages[0].Content = "changed"
	if _, err := db.Reserve(ctx, token, idem, changed); !errors.Is(err, store.ErrConflict) {
		t.Fatal("idempotency key accepted with different input")
	}
	reopened, err := store.Open(ctx, dsn, db.Network, key)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	j, err := reopened.Job(ctx, id, token, "", false)
	if err != nil || j.Request != nil || j.State != "reserved" {
		t.Fatalf("recovery/read isolation failed: %v", err)
	}
	badKey := bytes.Repeat([]byte{7}, 32)
	if wrong, err := store.Open(ctx, dsn, db.Network, badKey); err == nil {
		wrong.Close()
		t.Fatal("wrong encryption key accepted")
	}
	if err := reopened.Cancel(ctx, token, id); err != nil {
		t.Fatal(err)
	}
	if balance, _ := db.Balance(ctx, token); balance != 4096 {
		t.Fatal("cancellation lost or duplicated credits")
	}
	next, err := db.Reserve(ctx, token, protocol.RandomID(), canonical)
	if err != nil || next.Salt == details.Salt || next.Hash == details.Hash {
		t.Fatalf("separate identical requests reused private commitments: %v", err)
	}
}

func TestReceiptValidationAndObservation(t *testing.T) {
	db, _, _ := testutil.Database(t)
	m, key, _ := enroll(t, db, 1)
	token := grant(t, db)
	j, err := db.Reserve(ctx, token, protocol.RandomID(), request())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Claim(ctx, m.Provider); err != nil {
		t.Fatal(err)
	}
	if err := db.Append(ctx, m.Provider, j.ID, protocol.FakeText); err != nil {
		t.Fatal(err)
	}
	r := protocol.Receipt{Version: protocol.Version, Network: db.Network, Job: j.ID,
		RequestHash: j.Hash, Provider: m.Provider, Model: m.Model, Profile: m.Profile,
		OutputHash: protocol.OutputCommitment(j.Salt, protocol.FakeText), Simulation: true,
		Usage: protocol.Usage{PromptTokens: request().InputBudget(), CompletionTokens: 7, TotalTokens: request().InputBudget() + 7}}
	_, attacker, _ := ed25519.GenerateKey(rand.Reader)
	bad := protocol.SignReceipt(r, attacker)
	if db.Finish(ctx, m.Provider, j.ID, &bad) == nil {
		t.Fatal("forged receipt accepted")
	}
	r.RequestHash = protocol.RequestCommitment(protocol.RandomID(), *j.Request)
	bad = protocol.SignReceipt(r, key)
	if db.Finish(ctx, m.Provider, j.ID, &bad) == nil {
		t.Fatal("wrong request salt accepted")
	}
	r.RequestHash = j.Hash
	r.OutputHash = protocol.OutputCommitment(protocol.RandomID(), protocol.FakeText)
	bad = protocol.SignReceipt(r, key)
	if db.Finish(ctx, m.Provider, j.ID, &bad) == nil {
		t.Fatal("wrong output commitment accepted")
	}
	r.OutputHash = protocol.OutputCommitment(j.Salt, protocol.FakeText)
	good := protocol.SignReceipt(r, key)
	if err := db.Finish(ctx, m.Provider, j.ID, &good); err != nil {
		t.Fatal(err)
	}
	if err := db.Finish(ctx, m.Provider, j.ID, &good); err != nil {
		t.Fatal("identical completion not idempotent:", err)
	}
	tampered := good
	tampered.Receipt.Usage.TotalTokens++
	if db.Finish(ctx, m.Provider, j.ID, &tampered) == nil {
		t.Fatal("changed duplicate receipt accepted")
	}
	balance, err := db.Balance(ctx, token)
	if err != nil || balance != 4096-int64(r.Usage.TotalTokens) {
		t.Fatalf("settled balance wrong: %d %v", balance, err)
	}
	report, err := db.Audit(ctx, j.ID)
	if err != nil || report["simulation_replay_match"] != true || report["execution_proven"] != false {
		t.Fatalf("misleading audit report: %v %v", report, err)
	}
	if _, err := db.Job(ctx, j.ID, "other-account", "", false); !errors.Is(err, store.ErrAuth) {
		t.Fatal("another account can read private output")
	}
	if _, err := db.DB.Exec(ctx, "DELETE FROM events"); err == nil {
		t.Fatal("accounting journal is mutable")
	}
	if _, err := db.DB.Exec(ctx, "UPDATE jobs SET created=now()-interval '25 hours' WHERE id=$1", j.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Reap(ctx); err != nil {
		t.Fatal(err)
	}
	report, err = db.Audit(ctx, j.ID)
	if err != nil || report["evidence"] != "expired" || report["receipt_valid"] != true {
		t.Fatalf("retention changed signature validity or exposed evidence: %v %v", report, err)
	}
	if _, err := db.Reserve(ctx, token, "", request()); err == nil {
		t.Fatal("empty idempotency key accepted")
	}
}

func TestRunningCancellationAndLeaseExpiry(t *testing.T) {
	db, _, _ := testutil.Database(t)
	m, _, _ := enroll(t, db, 1)
	token := grant(t, db)
	j, err := db.Reserve(ctx, token, protocol.RandomID(), request())
	if err != nil {
		t.Fatal(err)
	}
	db.Claim(ctx, m.Provider)
	if err := db.Cancel(ctx, token, j.ID); err != nil {
		t.Fatal(err)
	}
	var slots int
	db.DB.QueryRow(ctx, "SELECT in_flight FROM providers").Scan(&slots)
	if slots != 1 {
		t.Fatal("running cancellation released capacity before provider acknowledgement")
	}
	if db.Append(ctx, m.Provider, j.ID, "late") == nil {
		t.Fatal("cancelled stream accepted late output")
	}
	if err := db.Finish(ctx, m.Provider, j.ID, nil); err != nil {
		t.Fatal(err)
	}
	next, err := db.Reserve(ctx, token, protocol.RandomID(), request())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(ctx, "UPDATE jobs SET deadline=$2 WHERE id=$1", next.ID, time.Now().Add(-time.Second).Unix()); err != nil {
		t.Fatal(err)
	}
	if err := db.Reap(ctx); err != nil {
		t.Fatal(err)
	}
	if balance, _ := db.Balance(ctx, token); balance != 4096 {
		t.Fatal("expiry did not conserve credits")
	}
	db.DB.QueryRow(ctx, "SELECT in_flight FROM providers").Scan(&slots)
	if slots != 0 {
		t.Fatal("expiry leaked a provider slot")
	}
}

func TestContributionCannotBeRepeated(t *testing.T) {
	db, _, _ := testutil.Database(t)
	m, _, _ := enroll(t, db, 1)
	if err := db.Fund(ctx, "contributor", 8192); err != nil {
		t.Fatal(err)
	}
	token, err := db.Contribute(ctx, m.Provider)
	if err != nil {
		t.Fatal(err)
	}
	if balance, _ := db.Balance(ctx, token); balance != 4096 {
		t.Fatal("missing contribution allowance")
	}
	if _, err := db.Contribute(ctx, m.Provider); !errors.Is(err, store.ErrConflict) {
		t.Fatal("traffic-free repeated contribution claim accepted")
	}
}

func TestPoolReclaimCannotRevokeGrantedCredits(t *testing.T) {
	db, _, _ := testutil.Database(t)
	enroll(t, db, 1)
	if err := db.Fund(ctx, "public", 8192); err != nil {
		t.Fatal(err)
	}
	token, err := db.Grant(ctx, "public", 4096)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Reallocate(ctx, "public", "paid", 8192); !errors.Is(err, store.ErrBudget) {
		t.Fatal("pool reclaim stole already allocated credits")
	}
	if err := db.Reallocate(ctx, "public", "paid", 4096); err != nil {
		t.Fatal(err)
	}
	if balance, _ := db.Balance(ctx, token); balance != 4096 {
		t.Fatal("reallocation changed a consumer entitlement")
	}
}

func TestWrongModelProfileCannotBorrowOtherCapacity(t *testing.T) {
	db, _, _ := testutil.Database(t)
	m, _, _ := enroll(t, db, 1)
	_, newKey, _ := ed25519.GenerateKey(rand.Reader)
	m.Model, m.ModelHash, m.Profile, m.Backend = "different", protocol.Hash([]byte("other")), "self-attested:vllm", "vllm"
	m.PublicKey = protocol.PublicKey(newKey)
	m.Provider = protocol.ProviderID(m.PublicKey)
	if _, err := db.Register(ctx, protocol.SignManifest(m, newKey)); err == nil {
		t.Fatal("different model can consume another profile's backing")
	}
}

func TestRevocationBlocksProviderWithoutReplenishingCapacity(t *testing.T) {
	db, _, _ := testutil.Database(t)
	m, _, token := enroll(t, db, 1)
	if err := db.Revoke(ctx, m.Provider); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ProviderAuth(ctx, token); !errors.Is(err, store.ErrAuth) {
		t.Fatal("revoked credential remains usable")
	}
	providers, err := db.Providers(ctx)
	if err != nil || len(providers) != 1 || providers[0].Active {
		t.Fatal("revoked provider remains eligible")
	}
}
