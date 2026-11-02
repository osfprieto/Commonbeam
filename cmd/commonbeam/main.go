package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/osfprieto/Commonbeam/internal/config"
	"github.com/osfprieto/Commonbeam/internal/control"
	"github.com/osfprieto/Commonbeam/internal/inference"
	"github.com/osfprieto/Commonbeam/internal/network"
	"github.com/osfprieto/Commonbeam/internal/protocol"
	"github.com/osfprieto/Commonbeam/internal/provider"
	"github.com/osfprieto/Commonbeam/internal/router"
	"github.com/osfprieto/Commonbeam/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "commonbeam:", err)
		os.Exit(1)
	}
}

func printJSON(w io.Writer, out any) error {
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	return e.Encode(out)
}

type session struct {
	Token string `json:"token"`
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Fprintln(out, `Commonbeam: local test network; no real payments, token, or verified AI.
Commands: init, join, dev, control, router, provider, contribute, session, models,
          chat, balance, job, cancel, drain, revoke, status, sponsor, grant, audit, doctor
Every command accepts --state (default .commonbeam). Use <command> --help.
For dev/control set COMMONBEAM_DATABASE_URL. No models are downloaded.`)
		return nil
	}
	command := args[0]
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	f.SetOutput(out)
	state := f.String("state", ".commonbeam", "local state directory (contains private credentials)")
	listen := f.String("listen", "", "explicit listen IP:port")
	authority := f.String("authority", "http://127.0.0.1:8081", "authority URL for init")
	routerURL := f.String("router", "http://127.0.0.1:8080", "router URL for init or client override")
	container := f.Bool("container-network", false, "allow container-internal non-loopback listen; publish only to host loopback")
	descriptor := f.String("descriptor", "", "signed network descriptor file for join")
	fingerprint := f.String("fingerprint", "", "trusted authority fingerprint for join")
	backendName := f.String("backend", "fake", "fake, vllm, or llamacpp")
	backendURL := f.String("backend-url", "http://127.0.0.1:8000/v1", "loopback model API base URL")
	model := f.String("model", protocol.FakeModel, "exact model ID")
	modelHash := f.String("model-hash", "", "approved model content hash (real backends)")
	name := f.String("name", "provider", "local provider state name")
	prompt := f.String("prompt", "", "text prompt; reads stdin when omitted")
	maxTokens := f.Int("max-tokens", 512, "maximum output tokens, 1-512")
	pool := f.String("pool", "public", "public, contributor, or paid")
	from := f.String("from", "", "source pool for unallocated test rebalancing")
	units := f.Int64("units", 4096, "test accounting units")
	jobID := f.String("job", "", "job identifier")
	providerID := f.String("provider-id", "", "provider identifier for administrator revocation")
	newSession := f.Bool("new-session", false, "explicitly replace a saved finite session")
	if err := f.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if command == "init" {
		fp, err := config.Init(*state, *authority, *routerURL)
		if err != nil {
			return err
		}
		return printJSON(out, map[string]string{"fingerprint": fp, "descriptor": filepath.Join(*state, "network.json"), "mode": "test_only"})
	}
	if command == "join" {
		var d protocol.SignedDescriptor
		if err := config.Read(*descriptor, &d); err != nil {
			return err
		}
		if err := d.Verify(*fingerprint); err != nil {
			return err
		}
		if err := config.Write(filepath.Join(*state, "trust.json"), map[string]string{"fingerprint": *fingerprint}, true); err != nil {
			return err
		}
		return config.Write(filepath.Join(*state, "network.json"), d, true)
	}
	if command == "doctor" {
		_, dockerErr := exec.LookPath("docker")
		_, gpuErr := exec.LookPath("nvidia-smi")
		return printJSON(out, map[string]any{"os": runtime.GOOS, "arch": runtime.GOARCH,
			"database_configured": os.Getenv("COMMONBEAM_DATABASE_URL") != "", "docker_available": dockerErr == nil,
			"nvidia_tool_available": gpuErr == nil, "model_downloads": "never_automatic",
			"qualification": "real_model_and_gpu_not_implied", "mode": "test_only"})
	}
	if command == "dev" {
		if _, err := os.Stat(filepath.Join(*state, "network.json")); errors.Is(err, os.ErrNotExist) {
			if _, err := config.Init(*state, *authority, *routerURL); err != nil {
				return err
			}
		}
	}
	d, err := config.Descriptor(*state)
	if err != nil {
		return err
	}
	base := d.Descriptor.Router
	f.Visit(func(fl *flag.Flag) {
		if fl.Name == "router" {
			base = *routerURL
		}
	})
	client := network.NewClient(base, "")
	switch command {
	case "session":
		if !*newSession {
			if _, err := os.Stat(filepath.Join(*state, "session.json")); err == nil {
				fmt.Fprintln(out, "Existing session retained; use balance, or --new-session to explicitly replace it.")
				return nil
			}
		}
		var s session
		if err := client.Call(ctx, "POST", "/commonbeam/v1/session", "", nil, &s); err != nil {
			return err
		}
		if err := config.Write(filepath.Join(*state, "session.json"), s, false); err != nil {
			return err
		}
		fmt.Fprintln(out, "Bounded test session saved; not a unique-person identity.")
		return nil
	case "status", "models":
		path := "/commonbeam/v1/status"
		if command == "models" {
			path = "/v1/models"
		}
		var result any
		if err := client.Call(ctx, "GET", path, "", nil, &result); err != nil {
			return err
		}
		return printJSON(out, result)
	case "balance", "chat", "job", "cancel":
		var s session
		if err := config.Read(filepath.Join(*state, "session.json"), &s); err != nil {
			return errors.New("run session first, or install an explicitly granted session.json")
		}
		client.Token = s.Token
		if command == "job" || command == "cancel" {
			if *jobID == "" {
				return errors.New("--job is required")
			}
			path, method := "/commonbeam/v1/jobs/"+*jobID, "GET"
			if command == "cancel" {
				path, method = path+"/cancel", "POST"
			}
			var result any
			if err := client.Call(ctx, method, path, "", nil, &result); err != nil {
				return err
			}
			return printJSON(out, result)
		}
		if command == "balance" {
			var result any
			if err := client.Call(ctx, "GET", "/commonbeam/v1/balance", "", nil, &result); err != nil {
				return err
			}
			return printJSON(out, result)
		}
		if *prompt == "" {
			b, err := io.ReadAll(io.LimitReader(os.Stdin, 4097))
			if err != nil {
				return err
			}
			*prompt = strings.TrimSpace(string(b))
		}
		return chat(ctx, client, protocol.ChatRequest{Model: *model, Messages: []protocol.Message{{Role: "user", Content: *prompt}}, Stream: true, MaxTokens: *maxTokens}, out)
	}
	secrets, err := config.LoadSecrets(*state)
	if err != nil && command != "provider" && command != "contribute" && command != "drain" {
		return errors.New("operator command requires scoped secrets.json; joining as a consumer does not grant operator privileges")
	}
	switch command {
	case "revoke":
		if *providerID == "" || strings.ContainsAny(*providerID, "/\\") {
			return errors.New("--provider-id is required")
		}
		c := network.NewClient(d.Descriptor.Authority, secrets.Admin)
		return c.Call(ctx, "POST", "/commonbeam/v1/revoke/"+*providerID, "", nil, nil)
	case "drain":
		if filepath.Base(*name) != *name {
			return errors.New("invalid provider name")
		}
		var saved providerState
		if err := config.Read(filepath.Join(*state, *name+"-provider.json"), &saved); err != nil {
			return err
		}
		c := network.NewClient(d.Descriptor.Authority, saved.Token)
		return c.Call(ctx, "POST", "/commonbeam/v1/drain", "", nil, nil)
	case "contribute":
		if !*newSession {
			if _, err := os.Stat(filepath.Join(*state, "session.json")); err == nil {
				return errors.New("saved session retained; use --new-session to explicitly select a contribution allowance")
			}
		}
		if filepath.Base(*name) != *name {
			return errors.New("invalid provider name")
		}
		var saved providerState
		if err := config.Read(filepath.Join(*state, *name+"-provider.json"), &saved); err != nil {
			return err
		}
		var result session
		c := network.NewClient(d.Descriptor.Authority, saved.Token)
		if err := c.Call(ctx, "POST", "/commonbeam/v1/contribute", "", nil, &result); err != nil {
			return err
		}
		if err := config.Write(filepath.Join(*state, "session.json"), result, false); err != nil {
			return err
		}
		fmt.Fprintln(out, "One-time test contribution allowance saved; not a hardware proof or cash reward.")
		return nil
	case "sponsor":
		c := network.NewClient(d.Descriptor.Authority, secrets.Sponsor)
		var result any
		path := "/commonbeam/v1/fund"
		payload := map[string]any{"pool": *pool, "units": *units}
		if *from != "" {
			path = "/commonbeam/v1/reallocate"
			payload = map[string]any{"from": *from, "to": *pool, "units": *units}
		}
		if err := c.Call(ctx, "POST", path, "", payload, &result); err != nil {
			return err
		}
		return printJSON(out, result)
	case "grant":
		c := network.NewClient(d.Descriptor.Authority, secrets.Admin)
		var result any
		if err := c.Call(ctx, "POST", "/commonbeam/v1/grant", "", map[string]any{"pool": *pool, "units": *units}, &result); err != nil {
			return err
		}
		return printJSON(out, result)
	case "audit":
		c := network.NewClient(d.Descriptor.Authority, secrets.Admin)
		var result any
		if *jobID == "" {
			return errors.New("--job is required")
		}
		if err := c.Call(ctx, "GET", "/commonbeam/v1/audit/"+*jobID, "", nil, &result); err != nil {
			return err
		}
		return printJSON(out, result)
	case "router":
		if secrets.Router == "" {
			return errors.New("COMMONBEAM_ROUTER_TOKEN or scoped secrets.json required")
		}
		if *listen == "" {
			*listen = "127.0.0.1:8080"
		}
		r := &router.Server{Authority: network.NewClient(d.Descriptor.Authority, secrets.Router)}
		fmt.Fprintln(out, "Test router listening on", *listen, "(plaintext at router/provider)")
		return network.Serve(ctx, *listen, r.Handler(), *container)
	case "provider":
		worker, err := enroll(ctx, *state, *name, secrets, d, *backendName, *backendURL, *model, *modelHash)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, "Test provider", worker.Manifest.Provider, "backend", worker.Manifest.Backend)
		return worker.Run(ctx)
	case "control", "dev":
		dsn := os.Getenv("COMMONBEAM_DATABASE_URL")
		if dsn == "" {
			return errors.New("COMMONBEAM_DATABASE_URL required; use local PostgreSQL or deploy/compose.yaml")
		}
		key, err := base64.StdEncoding.DecodeString(secrets.Evidence)
		if err != nil {
			return errors.New("invalid evidence key")
		}
		db, err := store.Open(ctx, dsn, d.Descriptor.Network, key)
		if err != nil {
			return errors.New("database startup failed; check connectivity, migrations, network identity and evidence key")
		}
		defer db.Close()
		s := &control.Server{Store: db, Admin: secrets.Admin, Router: secrets.Router, Enroll: secrets.Enroll, Sponsor: secrets.Sponsor}
		if command == "dev" {
			return dev(ctx, *state, d, secrets, s, out, *container)
		}
		if *listen == "" {
			*listen = "127.0.0.1:8081"
		}
		go s.Reconcile(ctx)
		fmt.Fprintln(out, "Single-authority test control plane listening on", *listen)
		return network.Serve(ctx, *listen, s.Handler(), *container)
	default:
		return errors.New("unknown command; use commonbeam help")
	}
}

type providerState struct {
	Token    string            `json:"token"`
	Network  string            `json:"network"`
	Manifest protocol.Manifest `json:"manifest"`
}

func enroll(ctx context.Context, dir, name string, secrets config.Secrets, d protocol.SignedDescriptor, backendName, backendURL, model, modelHash string) (*provider.Worker, error) {
	if filepath.Base(name) != name || name == "." || name == ".." {
		return nil, errors.New("provider name must be a simple filename")
	}
	key, err := config.ProviderKey(filepath.Join(dir, name+"-key.json"))
	if err != nil {
		return nil, err
	}
	var backend inference.Backend = inference.Fake{}
	profile := protocol.FakeProfile
	if backendName != "fake" {
		if backendName != "vllm" && backendName != "llamacpp" || model == protocol.FakeModel || modelHash == "" {
			return nil, errors.New("real backend requires vllm/llamacpp, --model and --model-hash; no automatic downloads")
		}
		h, err := inference.NewHTTP(backendURL, os.Getenv("COMMONBEAM_BACKEND_API_KEY"))
		if err != nil {
			return nil, err
		}
		if err := h.Probe(ctx, model); err != nil {
			return nil, err
		}
		backend, profile = h, "self-attested:"+backendName
	} else {
		model, modelHash = protocol.FakeModel, protocol.Hash([]byte(protocol.FakeText))
	}
	path := filepath.Join(dir, name+"-provider.json")
	var saved providerState
	if _, err := os.Stat(path); err == nil {
		if err := config.Read(path, &saved); err != nil {
			return nil, err
		}
		if saved.Network != d.Descriptor.Network || saved.Manifest.Provider != protocol.ProviderID(protocol.PublicKey(key)) ||
			saved.Manifest.Model != model || saved.Manifest.ModelHash != modelHash || saved.Manifest.Backend != backendName {
			return nil, errors.New("provider identity/backend mismatch; use a distinct provider name")
		}
		if saved.Manifest.Expires <= time.Now().Unix() {
			return nil, errors.New("provider epoch expired; use a new name and explicitly fund a new test epoch")
		}
	} else {
		m := protocol.Manifest{Version: protocol.Version, Network: d.Descriptor.Network,
			Provider: protocol.ProviderID(protocol.PublicKey(key)), Model: model, ModelHash: modelHash, Profile: profile,
			Backend: backendName, Capacity: 100000, Slots: 1, Expires: time.Now().Add(24 * time.Hour).Unix(), PublicKey: protocol.PublicKey(key)}
		var result struct {
			Token string `json:"token"`
		}
		c := network.NewClient(d.Descriptor.Authority, secrets.Enroll)
		if err := c.Call(ctx, "POST", "/commonbeam/v1/enroll", "", protocol.SignManifest(m, key), &result); err != nil {
			return nil, err
		}
		saved = providerState{result.Token, d.Descriptor.Network, m}
		if err := config.Write(path, saved, true); err != nil {
			return nil, err
		}
	}
	return &provider.Worker{Client: network.NewClient(d.Descriptor.Authority, saved.Token), Backend: backend, Key: ed25519.PrivateKey(key), Manifest: saved.Manifest}, nil
}

func dev(ctx context.Context, state string, d protocol.SignedDescriptor, secrets config.Secrets, controlServer *control.Server, out io.Writer, container bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errors := make(chan error, 4)
	var wg sync.WaitGroup
	start := func(fn func() error) {
		wg.Add(1)
		go func() { defer wg.Done(); errors <- fn() }()
	}
	authorityAddr := strings.TrimPrefix(d.Descriptor.Authority, "http://")
	routerAddr := strings.TrimPrefix(d.Descriptor.Router, "http://")
	if container {
		authorityAddr, routerAddr = "0.0.0.0:8081", "0.0.0.0:8080"
	}
	start(func() error { return network.Serve(ctx, authorityAddr, controlServer.Handler(), container) })
	go controlServer.Reconcile(ctx)
	c := network.NewClient(d.Descriptor.Authority, secrets.Router)
	ready := false
	for range 50 {
		if c.Call(ctx, "GET", "/healthz", "", nil, nil) == nil {
			ready = true
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errors:
			return err
		case <-time.After(100 * time.Millisecond):
		}
	}
	if !ready {
		return fmt.Errorf("authority did not become ready")
	}
	for _, name := range []string{"demo-one", "demo-two"} {
		w, err := enroll(ctx, state, name, secrets, d, "fake", "", protocol.FakeModel, "")
		if err != nil {
			return err
		}
		start(func() error { return w.Run(ctx) })
	}
	var seeded bool
	if err := controlServer.Store.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM metadata WHERE key='demo_seeded')").Scan(&seeded); err != nil {
		return err
	}
	if !seeded {
		for pool, units := range map[string]int64{"public": 50000, "contributor": 110000, "paid": 40000} {
			if err := controlServer.Store.Fund(ctx, pool, units); err != nil {
				return err
			}
		}
		if _, err := controlServer.Store.DB.Exec(ctx, "INSERT INTO metadata(key,value) VALUES('demo_seeded','true')"); err != nil {
			return err
		}
	}
	r := &router.Server{Authority: c}
	start(func() error { return network.Serve(ctx, routerAddr, r.Handler(), container) })
	fmt.Fprintln(out, "SIMULATED test network ready at", d.Descriptor.Router, "; no AI model, real payments, or independent operators.")
	select {
	case <-ctx.Done():
		cancel()
		wg.Wait()
		return nil
	case err := <-errors:
		cancel()
		wg.Wait()
		return err
	}
}

func chat(ctx context.Context, c *network.Client, request protocol.ChatRequest, out io.Writer) error {
	if err := request.Validate(); err != nil {
		return err
	}
	b, _ := json.Marshal(request)
	req, err := http.NewRequestWithContext(ctx, "POST", c.Base+"/v1/chat/completions", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	client := *c.HTTP
	client.Timeout = 130 * time.Second
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("chat endpoint unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("chat returned HTTP %d; inspect balance/status", resp.StatusCode)
	}
	scanner := bufio.NewScanner(io.LimitReader(resp.Body, 2<<20))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			fmt.Fprintln(out)
			return nil
		}
		var event struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Error json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return errors.New("invalid stream event")
		}
		if len(event.Error) > 0 {
			return fmt.Errorf("stream interrupted; job %s; query status before retrying", resp.Header.Get("X-Commonbeam-Job-Id"))
		}
		for _, choice := range event.Choices {
			fmt.Fprint(out, choice.Delta.Content)
		}
	}
	return errors.New("stream ended without completion")
}
