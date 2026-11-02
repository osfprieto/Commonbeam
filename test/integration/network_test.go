package integration_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/osfprieto/Commonbeam/internal/control"
	"github.com/osfprieto/Commonbeam/internal/inference"
	"github.com/osfprieto/Commonbeam/internal/network"
	"github.com/osfprieto/Commonbeam/internal/protocol"
	"github.com/osfprieto/Commonbeam/internal/provider"
	"github.com/osfprieto/Commonbeam/internal/router"
	"github.com/osfprieto/Commonbeam/internal/testutil"
)

func TestFourRolesAcrossIndependentRouters(t *testing.T) {
	db, _, _ := testutil.Database(t)
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	server := &control.Server{Store: db, Admin: protocol.RandomID(), Router: protocol.RandomID(),
		Enroll: protocol.RandomID(), Sponsor: protocol.RandomID()}
	authority := httptest.NewServer(server.Handler())
	t.Cleanup(authority.Close)
	authURL := authority.URL
	var workers []*provider.Worker
	for range 2 {
		_, key, _ := ed25519.GenerateKey(rand.Reader)
		m := protocol.Manifest{Version: protocol.Version, Network: db.Network,
			Provider: protocol.ProviderID(protocol.PublicKey(key)), Model: protocol.FakeModel,
			ModelHash: protocol.Hash([]byte(protocol.FakeText)), Profile: protocol.FakeProfile,
			Backend: "fake", Capacity: 100000, Slots: 1, Expires: time.Now().Add(time.Hour).Unix(),
			PublicKey: protocol.PublicKey(key)}
		c := network.NewClient(authURL, server.Enroll)
		var result struct {
			Token string `json:"token"`
		}
		if err := c.Call(ctx, "POST", "/commonbeam/v1/enroll", "", protocol.SignManifest(m, key), &result); err != nil {
			t.Fatal(err)
		}
		if err := c.Call(ctx, "POST", "/commonbeam/v1/enroll", "", protocol.SignManifest(m, key), &result); err == nil {
			t.Fatal("replayed enrollment obtained new provider credential")
		}
		w := &provider.Worker{Client: network.NewClient(authURL, result.Token), Backend: inference.Fake{}, Key: key, Manifest: m}
		workers = append(workers, w)
		wg.Add(1)
		go func() { defer wg.Done(); w.Run(ctx) }()
	}
	router1 := httptest.NewServer((&router.Server{Authority: network.NewClient(authURL, server.Router)}).Handler())
	router2 := httptest.NewServer((&router.Server{Authority: network.NewClient(authURL, server.Router)}).Handler())
	t.Cleanup(router1.Close)
	t.Cleanup(router2.Close)
	t.Cleanup(func() { cancel(); wg.Wait() })
	sponsor := network.NewClient(authURL, server.Sponsor)
	for pool, units := range map[string]int64{"public": 16384, "contributor": 8192, "paid": 4096} {
		if err := sponsor.Call(ctx, "POST", "/commonbeam/v1/fund", "", map[string]any{"pool": pool, "units": units}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if sponsor.Call(ctx, "POST", "/commonbeam/v1/grant", "", map[string]any{"pool": "public", "units": 4096}, nil) == nil {
		t.Fatal("sponsor credential can issue arbitrary account grants")
	}
	var contribution struct {
		Token string `json:"token"`
	}
	if err := workers[0].Client.Call(ctx, "POST", "/commonbeam/v1/contribute", "", nil, &contribution); err != nil {
		t.Fatal(err)
	}
	contributor := network.NewClient(router2.URL, contribution.Token)
	if err := contributor.Call(ctx, "GET", "/commonbeam/v1/balance", "", nil, new(any)); err != nil {
		t.Fatal("contribution not portable to second router:", err)
	}
	client := network.NewClient(router1.URL, "")
	var session struct {
		Token string `json:"token"`
	}
	if err := client.Call(ctx, "POST", "/commonbeam/v1/session", "", nil, &session); err != nil {
		t.Fatal(err)
	}
	client.Token = session.Token
	req := protocol.ChatRequest{Model: protocol.FakeModel, Messages: []protocol.Message{{Role: "user", Content: "hello"}}, MaxTokens: 8}
	idem := protocol.RandomID()
	var job string
	for _, routerURL := range []string{router1.URL, router2.URL} {
		b, _ := json.Marshal(req)
		httpReq, _ := http.NewRequestWithContext(ctx, "POST", routerURL+"/v1/chat/completions", strings.NewReader(string(b)))
		httpReq.Header.Set("Authorization", "Bearer "+session.Token)
		httpReq.Header.Set("Idempotency-Key", idem)
		httpReq.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 || !strings.Contains(string(body), protocol.FakeText) {
			t.Fatalf("chat failed: status=%d body=%s", resp.StatusCode, body)
		}
		id := resp.Header.Get("X-Commonbeam-Job-Id")
		if job != "" && job != id {
			t.Fatal("router switch broke idempotency")
		}
		job = id
	}
	var count int
	db.DB.QueryRow(ctx, "SELECT count(*) FROM jobs").Scan(&count)
	if count != 1 {
		t.Fatal("same request charged twice across routers")
	}
	req.Stream = true
	b, _ := json.Marshal(req)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", router2.URL+"/v1/chat/completions", strings.NewReader(string(b)))
	httpReq.Header.Set("Authorization", "Bearer "+session.Token)
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatal(err)
	}
	stream, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(stream), "[DONE]") || !strings.Contains(string(stream), `"finish_reason":"stop"`) {
		t.Fatalf("incomplete stream: %s", stream)
	}
	admin := network.NewClient(authURL, server.Admin)
	var report map[string]any
	if err := admin.Call(ctx, "GET", "/commonbeam/v1/audit/"+job, "", nil, &report); err != nil {
		t.Fatal(err)
	}
	if report["simulation_replay_match"] != true || report["execution_proven"] != false {
		t.Fatalf("invalid audit: %v", report)
	}
	for _, body := range []string{
		`{"model":"commonbeam-simulated","messages":[{"role":"user","content":"x"}],"tools":[]}`,
		`{"model":"commonbeam-simulated","messages":[{"role":"user","content":"x"}],"max_tokens":9000}`,
	} {
		req, _ := http.NewRequestWithContext(ctx, "POST", router1.URL+"/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+session.Token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("unsupported request accepted: %d", resp.StatusCode)
		}
	}
	router1.Close()
	if err := workers[0].Client.Call(ctx, "POST", "/commonbeam/v1/drain", "", nil, nil); err != nil {
		t.Fatal(err)
	}
	req.Stream = false
	var completion any
	if err := contributor.Call(ctx, "POST", "/v1/chat/completions", "", req, &completion); err != nil {
		t.Fatal("contribution not redeemable with another provider after router failure:", err)
	}
	status, err := db.Status(ctx)
	if err != nil || status["test_network"] != true {
		t.Fatal("aggregate accounting status unavailable:", err)
	}
	cancel()
	wg.Wait()
	authority.Close()
	client.Base = router2.URL
	if err := client.Call(context.Background(), "GET", "/v1/models", "", nil, new(any)); err == nil {
		t.Fatal("router pretended authority-independent service after authority failure")
	} else if api, ok := err.(*network.APIError); !ok || api.Status != 503 {
		t.Fatal("authority failure was not reported as service unavailability:", err)
	}
}
