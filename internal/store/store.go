package store

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/osfprieto/Commonbeam/internal/protocol"
)

//go:embed schema.sql
var schema string

var (
	ErrAuth     = errors.New("invalid or unauthorized credential")
	ErrBudget   = errors.New("test funding or credit balance exhausted")
	ErrCapacity = errors.New("eligible provider capacity unavailable")
	ErrConflict = errors.New("request conflicts with existing state")
	ErrMissing  = errors.New("resource not found")
)

type Store struct {
	DB      *pgxpool.Pool
	Network string
	aead    cipher.AEAD
}

func Open(ctx context.Context, dsn, network string, key []byte) (*Store, error) {
	if network == "" || len(key) != 32 {
		return nil, errors.New("network and 32-byte evidence key required")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("invalid database configuration")
	}
	cfg.MaxConns = 16
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("database connection failed")
	}
	block, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(block)
	s := &Store{db, network, aead}
	tx, err := s.begin(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, schema); err == nil {
		for k, v := range map[string]string{"network": network, "evidence_key_hash": protocol.Hash(key), "schema": "1"} {
			_, err = tx.Exec(ctx, "INSERT INTO metadata(key,value) VALUES($1,$2) ON CONFLICT DO NOTHING", k, v)
			if err != nil {
				break
			}
			var actual string
			err = tx.QueryRow(ctx, "SELECT value FROM metadata WHERE key=$1", k).Scan(&actual)
			if err != nil || actual != v {
				err = errors.New("database network, schema or evidence key mismatch")
				break
			}
		}
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		tx.Rollback(ctx)
		db.Close()
		return nil, err
	}
	return s, nil
}

// The pilot deliberately serializes accounting mutations across all authority instances.
func (s *Store) begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(734927841)"); err != nil {
		tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}

func (s *Store) encrypt(b []byte) []byte {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		panic(err)
	}
	return s.aead.Seal(nonce, nonce, b, []byte(s.Network))
}

func (s *Store) decrypt(b []byte) ([]byte, error) {
	n := s.aead.NonceSize()
	if len(b) < n {
		return nil, errors.New("invalid encrypted evidence")
	}
	return s.aead.Open(nil, b[:n], b[n:], []byte(s.Network))
}

func event(ctx context.Context, tx pgx.Tx, kind, subject string, units int64) error {
	_, err := tx.Exec(ctx, "INSERT INTO events(kind,subject,units) VALUES($1,$2,$3)", kind, subject, units)
	return err
}

func (s *Store) Register(ctx context.Context, m protocol.SignedManifest) (string, error) {
	if err := m.Validate(s.Network, time.Now()); err != nil {
		return "", err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM providers WHERE id=$1)", m.Manifest.Provider).Scan(&exists); err != nil {
		return "", err
	}
	if exists {
		return "", ErrConflict
	}
	for k, value := range map[string]string{
		"service_model": m.Manifest.Model, "service_model_hash": m.Manifest.ModelHash,
		"service_profile": m.Manifest.Profile,
	} {
		if _, err := tx.Exec(ctx, "INSERT INTO metadata(key,value) VALUES($1,$2) ON CONFLICT DO NOTHING", k, value); err != nil {
			return "", err
		}
		var actual string
		if err := tx.QueryRow(ctx, "SELECT value FROM metadata WHERE key=$1", k).Scan(&actual); err != nil {
			return "", err
		}
		if actual != value {
			return "", errors.New("one model/hash/profile per test network; use a separate network for another profile")
		}
	}
	if _, err := tx.Exec(ctx, "INSERT INTO metadata(key,value) VALUES('epoch_expires',$1) ON CONFLICT DO NOTHING", strconv.FormatInt(m.Manifest.Expires, 10)); err != nil {
		return "", err
	}
	var epochString string
	if err := tx.QueryRow(ctx, "SELECT value FROM metadata WHERE key='epoch_expires'").Scan(&epochString); err != nil {
		return "", err
	}
	epoch, err := strconv.ParseInt(epochString, 10, 64)
	if err != nil || epoch <= time.Now().Unix() || m.Manifest.Expires < epoch {
		return "", errors.New("test epoch expired or manifest does not cover it; create an explicitly new network")
	}
	token := protocol.RandomID()
	b, _ := json.Marshal(m)
	_, err = tx.Exec(ctx, "INSERT INTO providers(id,token_hash,manifest,available,slots,expires) VALUES($1,$2,$3,$4,$5,$6)",
		m.Manifest.Provider, protocol.Hash([]byte(token)), b, m.Manifest.Capacity, m.Manifest.Slots, epoch)
	if err == nil {
		err = event(ctx, tx, "enroll_test_capacity", m.Manifest.Provider, m.Manifest.Capacity)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	return token, err
}

func (s *Store) ProviderAuth(ctx context.Context, token string) (string, error) {
	var id string
	err := s.DB.QueryRow(ctx, "SELECT id FROM providers WHERE token_hash=$1", protocol.Hash([]byte(token))).Scan(&id)
	if err != nil {
		return "", ErrAuth
	}
	return id, nil
}

func (s *Store) Heartbeat(ctx context.Context, provider string) error {
	tag, err := s.DB.Exec(ctx, "UPDATE providers SET heartbeat=now() WHERE id=$1 AND expires>$2", provider, time.Now().Unix())
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrMissing
	}
	return err
}

func (s *Store) Drain(ctx context.Context, provider string) error {
	_, err := s.DB.Exec(ctx, "UPDATE providers SET active=false WHERE id=$1", provider)
	return err
}

func (s *Store) Resume(ctx context.Context, provider string) error {
	tag, err := s.DB.Exec(ctx, "UPDATE providers SET active=true,heartbeat=now() WHERE id=$1 AND expires>$2", provider, time.Now().Unix())
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrConflict
	}
	return err
}

func (s *Store) Revoke(ctx context.Context, provider string) error {
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, "UPDATE providers SET active=false,token_hash=$2 WHERE id=$1", provider, protocol.Hash([]byte(protocol.RandomID())))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrMissing
	}
	if err = event(ctx, tx, "revoke_provider", provider, 0); err == nil {
		err = tx.Commit(ctx)
	}
	return err
}

type Provider struct {
	ID        string                  `json:"id"`
	Signed    protocol.SignedManifest `json:"signed"`
	Available int64                   `json:"available"`
	InFlight  int                     `json:"in_flight"`
	Active    bool                    `json:"active"`
}

func (s *Store) Providers(ctx context.Context) ([]Provider, error) {
	rows, err := s.DB.Query(ctx, "SELECT id,manifest,available,in_flight,active AND expires>$1 AND heartbeat>now()-interval '30 seconds' FROM providers ORDER BY id", time.Now().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Provider{}
	for rows.Next() {
		var p Provider
		var b []byte
		if err := rows.Scan(&p.ID, &b, &p.Available, &p.InFlight, &p.Active); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &p.Signed); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func validPool(pool string) bool { return pool == "public" || pool == "contributor" || pool == "paid" }

func (s *Store) Fund(ctx context.Context, pool string, units int64) error {
	if !validPool(pool) || units < 1 || units > 1_000_000_000 {
		return errors.New("invalid test funding")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var capacity, credits int64
	err = tx.QueryRow(ctx, "SELECT COALESCE(sum(available),0) FROM providers WHERE active AND expires>$1 AND heartbeat>now()-interval '30 seconds'", time.Now().Unix()).Scan(&capacity)
	if err == nil {
		err = tx.QueryRow(ctx, "SELECT (SELECT COALESCE(sum(balance),0) FROM pools)+(SELECT COALESCE(sum(balance),0) FROM accounts)").Scan(&credits)
	}
	if err != nil {
		return err
	}
	if units > capacity || credits > capacity-units {
		return ErrCapacity
	}
	_, err = tx.Exec(ctx, "UPDATE pools SET balance=balance+$2 WHERE name=$1", pool, units)
	if err == nil {
		err = event(ctx, tx, "test_funding", pool, units)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	return err
}

func (s *Store) Grant(ctx context.Context, pool string, units int64) (string, error) {
	if !validPool(pool) || units < 1 || units > 4096 {
		return "", errors.New("grant must contain 1-4096 test units")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var epochValid bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM metadata WHERE key='epoch_expires' AND value::bigint>$1)", time.Now().Unix()).Scan(&epochValid); err != nil {
		return "", err
	}
	if !epochValid {
		return "", ErrCapacity
	}
	tag, err := tx.Exec(ctx, "UPDATE pools SET balance=balance-$2 WHERE name=$1 AND balance>=$2", pool, units)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() != 1 {
		return "", ErrBudget
	}
	token := protocol.RandomID()
	id := protocol.Hash([]byte(token))
	_, err = tx.Exec(ctx, "INSERT INTO accounts(id,pool,balance) VALUES($1,$2,$3)", id, pool, units)
	if err == nil {
		err = event(ctx, tx, "grant", id, units)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	return token, err
}

func (s *Store) Balance(ctx context.Context, token string) (int64, error) {
	var balance int64
	err := s.DB.QueryRow(ctx, "SELECT balance FROM accounts WHERE id=$1", protocol.Hash([]byte(token))).Scan(&balance)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrAuth
	}
	if err == nil {
		var valid bool
		err = s.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM metadata WHERE key='epoch_expires' AND value::bigint>$1)", time.Now().Unix()).Scan(&valid)
		if err == nil && !valid {
			return 0, ErrBudget
		}
	}
	return balance, err
}

const jobColumns = "id,state,provider_id,request_hash,output,reserved,cost,deadline,receipt,audit,request,account_id"

type scanner interface{ Scan(...any) error }

func (s *Store) scanJob(row scanner) (protocol.Job, string, error) {
	var j protocol.Job
	var output, request, receipt []byte
	var account string
	err := row.Scan(&j.ID, &j.State, &j.Provider, &j.Hash, &output, &j.Reserved, &j.Cost, &j.Deadline, &receipt, &j.Audit, &request, &account)
	if errors.Is(err, pgx.ErrNoRows) {
		return j, "", ErrMissing
	}
	if err != nil {
		return j, "", err
	}
	b, err := s.decrypt(output)
	if err != nil {
		return j, "", err
	}
	j.Output = string(b)
	b, err = s.decrypt(request)
	if err != nil {
		return j, "", err
	}
	var envelope struct {
		Request *protocol.ChatRequest `json:"request"`
		Salt    string                `json:"salt"`
	}
	if err = json.Unmarshal(b, &envelope); err != nil {
		return j, "", err
	}
	j.Request, j.Salt = envelope.Request, envelope.Salt
	// Read early local demonstration records; new records always use private salts.
	if j.Request == nil && string(b) != "null" {
		var legacy protocol.ChatRequest
		if err := json.Unmarshal(b, &legacy); err != nil {
			return j, "", err
		}
		if legacy.Model != "" {
			j.Request = &legacy
		}
	}
	j.EvidenceAvailable = j.Request != nil
	if len(receipt) > 0 {
		err = json.Unmarshal(receipt, &j.Receipt)
	}
	return j, account, err
}

func (s *Store) Reserve(ctx context.Context, token, idem string, request protocol.ChatRequest) (protocol.Job, error) {
	if err := request.Validate(); err != nil {
		return protocol.Job{}, err
	}
	if len(idem) < 8 || len(idem) > 128 {
		return protocol.Job{}, errors.New("idempotency key must be 8-128 characters")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return protocol.Job{}, err
	}
	defer tx.Rollback(ctx)
	account := protocol.Hash([]byte(token))
	var balance int64
	if err = tx.QueryRow(ctx, "SELECT balance FROM accounts WHERE id=$1", account).Scan(&balance); err != nil {
		return protocol.Job{}, ErrAuth
	}
	old, _, err := s.scanJob(tx.QueryRow(ctx, "SELECT "+jobColumns+" FROM jobs WHERE account_id=$1 AND idem=$2", account, idem))
	if err == nil {
		if old.Request == nil || old.Request.Hash() != request.Hash() {
			return protocol.Job{}, ErrConflict
		}
		if !old.EvidenceAvailable {
			return protocol.Job{}, ErrConflict
		}
		return old, tx.Commit(ctx)
	}
	if !errors.Is(err, ErrMissing) {
		return protocol.Job{}, err
	}
	cost := int64(request.Budget())
	if balance < cost {
		return protocol.Job{}, ErrBudget
	}
	var busy bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM jobs WHERE account_id=$1 AND state IN ('reserved','accepted','streaming','cancelling'))", account).Scan(&busy); err != nil {
		return protocol.Job{}, err
	}
	if busy {
		return protocol.Job{}, ErrConflict
	}
	var provider string
	err = tx.QueryRow(ctx, `SELECT id FROM providers WHERE active AND expires>$1
		AND heartbeat>now()-interval '30 seconds' AND available>=$2 AND in_flight<slots
		AND manifest->'manifest'->>'model'=$3 ORDER BY in_flight,id LIMIT 1`,
		time.Now().Unix(), cost, request.Model).Scan(&provider)
	if errors.Is(err, pgx.ErrNoRows) {
		return protocol.Job{}, ErrCapacity
	}
	if err != nil {
		return protocol.Job{}, err
	}
	j := protocol.Job{ID: protocol.RandomID(), State: "reserved", Provider: provider,
		Request: &request, Reserved: cost, Deadline: time.Now().Add(2 * time.Minute).Unix(),
		EvidenceAvailable: true, Salt: protocol.RandomID()}
	j.Hash = protocol.RequestCommitment(j.Salt, request)
	input, _ := json.Marshal(struct {
		Request protocol.ChatRequest `json:"request"`
		Salt    string               `json:"salt"`
	}{request, j.Salt})
	_, err = tx.Exec(ctx, `INSERT INTO jobs(id,account_id,idem,request_hash,provider_id,request,output,reserved,state,deadline)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,'reserved',$9)`, j.ID, account, idem, j.Hash, provider, s.encrypt(input), s.encrypt(nil), cost, j.Deadline)
	if err == nil {
		_, err = tx.Exec(ctx, "UPDATE accounts SET balance=balance-$2 WHERE id=$1", account, cost)
	}
	if err == nil {
		_, err = tx.Exec(ctx, "UPDATE providers SET available=available-$2,in_flight=in_flight+1 WHERE id=$1", provider, cost)
	}
	if err == nil {
		err = event(ctx, tx, "reserve", j.ID, cost)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	return j, err
}

func (s *Store) Claim(ctx context.Context, provider string) (*protocol.Job, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	j, _, err := s.scanJob(tx.QueryRow(ctx, "SELECT "+jobColumns+" FROM jobs WHERE provider_id=$1 AND state='reserved' AND deadline>$2 ORDER BY created LIMIT 1", provider, time.Now().Unix()))
	if errors.Is(err, ErrMissing) {
		return nil, tx.Commit(ctx)
	}
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, "UPDATE jobs SET state='accepted' WHERE id=$1", j.ID)
	if err == nil {
		err = tx.Commit(ctx)
	}
	j.State = "accepted"
	return &j, err
}

func (s *Store) Job(ctx context.Context, id, accountToken, provider string, admin bool) (protocol.Job, error) {
	j, account, err := s.scanJob(s.DB.QueryRow(ctx, "SELECT "+jobColumns+" FROM jobs WHERE id=$1", id))
	if err != nil {
		return protocol.Job{}, err
	}
	if !admin && (provider == "" || j.Provider != provider) && account != protocol.Hash([]byte(accountToken)) {
		return protocol.Job{}, ErrAuth
	}
	if !admin && provider == "" {
		j.Request = nil
	}
	return j, nil
}

func (s *Store) Append(ctx context.Context, provider, id, text string) error {
	if len(text) > 4096 {
		return errors.New("stream chunk exceeds 4096 bytes")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	j, _, err := s.scanJob(tx.QueryRow(ctx, "SELECT "+jobColumns+" FROM jobs WHERE id=$1", id))
	if err != nil {
		return err
	}
	if j.Provider != provider {
		return ErrAuth
	}
	if j.State != "accepted" && j.State != "streaming" || j.Deadline <= time.Now().Unix() {
		return ErrConflict
	}
	if len(j.Output)+len(text) > 32768 {
		return errors.New("output exceeds test policy")
	}
	_, err = tx.Exec(ctx, "UPDATE jobs SET output=$2,state='streaming' WHERE id=$1", id, s.encrypt([]byte(j.Output+text)))
	if err == nil {
		err = tx.Commit(ctx)
	}
	return err
}

func (s *Store) release(ctx context.Context, tx pgx.Tx, j protocol.Job, account, state string, cost int64, receipt *protocol.SignedReceipt) error {
	if cost < 0 || cost > j.Reserved {
		return errors.New("usage outside reserved budget")
	}
	var b []byte
	if receipt != nil {
		b, _ = json.Marshal(receipt)
	}
	var sample [1]byte
	if _, err := rand.Read(sample[:]); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "UPDATE jobs SET state=$2,cost=$3,receipt=$4,audit=$5 WHERE id=$1", j.ID, state, cost, b, state == "complete" && sample[0] < 64)
	if err == nil {
		_, err = tx.Exec(ctx, "UPDATE accounts SET balance=balance+$2 WHERE id=$1", account, j.Reserved-cost)
	}
	if err == nil {
		_, err = tx.Exec(ctx, "UPDATE providers SET available=available+$2,in_flight=in_flight-1 WHERE id=$1", j.Provider, j.Reserved-cost)
	}
	if err == nil {
		err = event(ctx, tx, state, j.ID, cost)
	}
	return err
}

func (s *Store) Finish(ctx context.Context, provider, id string, receipt *protocol.SignedReceipt) error {
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	j, account, err := s.scanJob(tx.QueryRow(ctx, "SELECT "+jobColumns+" FROM jobs WHERE id=$1", id))
	if err != nil {
		return err
	}
	if j.Provider != provider {
		return ErrAuth
	}
	if protocol.Terminal(j.State) {
		if j.State == "complete" && receipt != nil && j.Receipt != nil && *receipt == *j.Receipt {
			return tx.Commit(ctx)
		}
		return ErrConflict
	}
	state, cost := "failed", int64(0)
	if j.State == "cancelling" {
		state, receipt = "cancelled", nil
	} else if receipt != nil {
		if j.State != "accepted" && j.State != "streaming" || j.Deadline <= time.Now().Unix() {
			return ErrConflict
		}
		var b []byte
		if err = tx.QueryRow(ctx, "SELECT manifest FROM providers WHERE id=$1", provider).Scan(&b); err != nil {
			return err
		}
		var m protocol.SignedManifest
		if err = json.Unmarshal(b, &m); err != nil {
			return err
		}
		r := receipt.Receipt
		if !receipt.Verify(m.Manifest.PublicKey) || r.Network != s.Network || r.Job != id ||
			r.Provider != provider || r.RequestHash != j.Hash || r.Model != j.Request.Model ||
			r.Profile != m.Manifest.Profile || r.OutputHash != protocol.OutputCommitment(j.Salt, j.Output) ||
			r.Simulation != (m.Manifest.Backend == "fake") ||
			r.Usage.PromptTokens < 0 || r.Usage.CompletionTokens < 0 ||
			r.Usage.CompletionTokens > j.Request.MaxTokens ||
			r.Usage.TotalTokens != r.Usage.PromptTokens+r.Usage.CompletionTokens {
			return errors.New("invalid receipt or usage")
		}
		state, cost = "complete", int64(r.Usage.TotalTokens)
	}
	if err = s.release(ctx, tx, j, account, state, cost, receipt); err == nil {
		err = tx.Commit(ctx)
	}
	return err
}

func (s *Store) Cancel(ctx context.Context, token, id string) error {
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	j, account, err := s.scanJob(tx.QueryRow(ctx, "SELECT "+jobColumns+" FROM jobs WHERE id=$1", id))
	if err != nil {
		return err
	}
	if account != protocol.Hash([]byte(token)) {
		return ErrAuth
	}
	if protocol.Terminal(j.State) {
		return tx.Commit(ctx)
	}
	if j.State == "reserved" {
		err = s.release(ctx, tx, j, account, "cancelled", 0, nil)
	} else {
		_, err = tx.Exec(ctx, "UPDATE jobs SET state='cancelling' WHERE id=$1", id)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	return err
}

func (s *Store) Reap(ctx context.Context) error {
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, "SELECT "+jobColumns+" FROM jobs WHERE state IN ('reserved','accepted','streaming','cancelling') AND deadline<=$1", time.Now().Unix())
	if err != nil {
		return err
	}
	type expired struct {
		job     protocol.Job
		account string
	}
	var jobs []expired
	for rows.Next() {
		j, account, err := s.scanJob(rows)
		if err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, expired{j, account})
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, e := range jobs {
		if err = s.release(ctx, tx, e.job, e.account, "expired", 0, nil); err != nil {
			return err
		}
	}
	// Logical retention does not promise erasure from PostgreSQL WAL or backups.
	_, err = tx.Exec(ctx, `UPDATE jobs SET request=$1,output=$2,evidence_retained=false
		WHERE state IN ('complete','failed','cancelled','expired')
		AND evidence_retained AND created<now()-interval '24 hours'`,
		s.encrypt([]byte("null")), s.encrypt(nil))
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) Status(ctx context.Context) (map[string]any, error) {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, "SELECT name,balance FROM pools ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pools := map[string]int64{}
	for rows.Next() {
		var name string
		var units int64
		if err = rows.Scan(&name, &units); err != nil {
			return nil, err
		}
		pools[name] = units
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	var epoch string
	err = tx.QueryRow(ctx, "SELECT value FROM metadata WHERE key='epoch_expires'").Scan(&epoch)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	var completed, spent, held, accountBalances int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE state='complete'),
		COALESCE(sum(cost),0),COALESCE(sum(reserved) FILTER(WHERE state IN ('reserved','accepted','streaming','cancelling')),0)
		FROM jobs`).Scan(&completed, &spent, &held); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(ctx, "SELECT COALESCE(sum(balance),0) FROM accounts").Scan(&accountBalances); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return map[string]any{"network": s.Network, "test_network": true, "funding": "test_only",
		"privacy": "router_and_provider_plaintext", "settlement": "single_authority",
		"pools": pools, "verification": "observation_only", "epoch_expires": epoch,
		"metrics": map[string]int64{"completed_jobs": completed, "consumed_test_units": spent,
			"reserved_test_units": held, "account_test_units": accountBalances}}, nil
}

func (s *Store) Audit(ctx context.Context, id string) (map[string]any, error) {
	j, err := s.Job(ctx, id, "", "", true)
	if err != nil {
		return nil, err
	}
	var b []byte
	if err = s.DB.QueryRow(ctx, "SELECT manifest FROM providers WHERE id=$1", j.Provider).Scan(&b); err != nil {
		return nil, err
	}
	var m protocol.SignedManifest
	if err = json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	valid := j.Receipt != nil && j.Receipt.Verify(m.Manifest.PublicKey)
	outputMatches := valid && j.EvidenceAvailable && j.Receipt.Receipt.OutputHash == protocol.OutputCommitment(j.Salt, j.Output)
	result := map[string]any{"job": id, "selected": j.Audit, "receipt_valid": valid,
		"output_commitment_matches": outputMatches, "execution_proven": false, "financial_penalty": false}
	if !j.EvidenceAvailable {
		result["evidence"] = "expired"
	} else if m.Manifest.Backend == "fake" {
		words := []string{"Commonbeam", "simulation.", "No", "AI", "model", "was", "executed."}
		n := len(words)
		if j.Request.MaxTokens < n {
			n = j.Request.MaxTokens
		}
		expected := ""
		for i, w := range words[:n] {
			if i > 0 {
				expected += " "
			}
			expected += w
		}
		result["simulation_replay_match"] = outputMatches && j.Output == expected
	} else {
		result["real_replay"] = "blocked_pending_qualified_hardware_and_model"
	}
	return result, nil
}

func (s *Store) Close() { s.DB.Close() }

func (s *Store) String() string { return fmt.Sprintf("Commonbeam test authority %s", s.Network) }

func (s *Store) Contribute(ctx context.Context, provider string) (string, error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var eligible bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM providers
		WHERE id=$1 AND active AND expires>$2 AND heartbeat>now()-interval '30 seconds')`,
		provider, time.Now().Unix()).Scan(&eligible); err != nil {
		return "", err
	}
	if !eligible {
		return "", ErrCapacity
	}
	tag, err := tx.Exec(ctx, "INSERT INTO metadata(key,value) VALUES($1,'claimed') ON CONFLICT DO NOTHING", "contribution:"+provider)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() == 0 {
		return "", ErrConflict
	}
	tag, err = tx.Exec(ctx, "UPDATE pools SET balance=balance-4096 WHERE name='contributor' AND balance>=4096")
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() == 0 {
		return "", ErrBudget
	}
	token := protocol.RandomID()
	id := protocol.Hash([]byte(token))
	_, err = tx.Exec(ctx, "INSERT INTO accounts(id,pool,balance) VALUES($1,'contributor',4096)", id)
	if err == nil {
		err = event(ctx, tx, "test_contribution_grant", provider, 4096)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	return token, err
}

func (s *Store) Reallocate(ctx context.Context, from, to string, units int64) error {
	if !validPool(from) || !validPool(to) || from == to || units < 1 || units > 1_000_000_000 {
		return errors.New("invalid pool reallocation")
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, "UPDATE pools SET balance=balance-$2 WHERE name=$1 AND balance>=$2", from, units)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrBudget
	}
	_, err = tx.Exec(ctx, "UPDATE pools SET balance=balance+$2 WHERE name=$1", to, units)
	if err == nil {
		err = event(ctx, tx, "reallocate:"+from+":"+to, "unallocated_test_units", units)
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	return err
}
