package control

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/osfprieto/Commonbeam/internal/network"
	"github.com/osfprieto/Commonbeam/internal/protocol"
	"github.com/osfprieto/Commonbeam/internal/store"
)

type Server struct {
	Store                          *store.Store
	Admin, Router, Enroll, Sponsor string
}

func (s *Server) auth(w http.ResponseWriter, r *http.Request, token string) bool {
	if !network.Authorized(r, token) {
		network.WriteError(w, 401, "invalid scoped credential", "authentication_error")
		return false
	}
	return true
}

func report(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	status, code := 400, "invalid_request_error"
	switch {
	case errors.Is(err, store.ErrAuth):
		status, code = 401, "authentication_error"
	case errors.Is(err, store.ErrBudget):
		status, code = 402, "funding_exhausted"
	case errors.Is(err, store.ErrCapacity):
		status, code = 503, "capacity_unavailable"
	case errors.Is(err, store.ErrConflict):
		status, code = 409, "state_conflict"
	case errors.Is(err, store.ErrMissing):
		status, code = 404, "not_found"
	default:
		// Database and transport internals may contain sensitive configuration.
		network.WriteError(w, 400, "operation rejected; check policy, evidence, and prerequisites", code)
		return true
	}
	network.WriteError(w, status, err.Error(), code)
	return true
}

func (s *Server) provider(w http.ResponseWriter, r *http.Request) string {
	id, err := s.Store.ProviderAuth(r.Context(), network.Bearer(r))
	if report(w, err) {
		return ""
	}
	return id
}

func (s *Server) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		if s.Store.DB.Ping(ctx) != nil {
			network.WriteError(w, 503, "accounting authority unavailable", "authority_unavailable")
			return
		}
		network.WriteJSON(w, 200, map[string]any{"ok": true, "test_network": true})
	})
	m.HandleFunc("GET /commonbeam/v1/status", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.Store.Status(r.Context())
		if !report(w, err) {
			network.WriteJSON(w, 200, out)
		}
	})
	m.HandleFunc("GET /commonbeam/v1/providers", func(w http.ResponseWriter, r *http.Request) {
		out, err := s.Store.Providers(r.Context())
		if !report(w, err) {
			network.WriteJSON(w, 200, out)
		}
	})
	m.HandleFunc("POST /commonbeam/v1/enroll", func(w http.ResponseWriter, r *http.Request) {
		if !s.auth(w, r, s.Enroll) {
			return
		}
		var manifest protocol.SignedManifest
		if !network.ReadJSON(w, r, &manifest) {
			return
		}
		token, err := s.Store.Register(r.Context(), manifest)
		if !report(w, err) {
			network.WriteJSON(w, 201, map[string]string{"token": token})
		}
	})
	m.HandleFunc("POST /commonbeam/v1/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		id := s.provider(w, r)
		if id != "" && !report(w, s.Store.Heartbeat(r.Context(), id)) {
			network.WriteJSON(w, 200, map[string]bool{"ok": true})
		}
	})
	m.HandleFunc("POST /commonbeam/v1/drain", func(w http.ResponseWriter, r *http.Request) {
		id := s.provider(w, r)
		if id != "" && !report(w, s.Store.Drain(r.Context(), id)) {
			network.WriteJSON(w, 200, map[string]bool{"draining": true})
		}
	})
	m.HandleFunc("POST /commonbeam/v1/resume", func(w http.ResponseWriter, r *http.Request) {
		id := s.provider(w, r)
		if id != "" && !report(w, s.Store.Resume(r.Context(), id)) {
			network.WriteJSON(w, 200, map[string]bool{"active": true})
		}
	})
	m.HandleFunc("POST /commonbeam/v1/contribute", func(w http.ResponseWriter, r *http.Request) {
		id := s.provider(w, r)
		if id == "" {
			return
		}
		token, err := s.Store.Contribute(r.Context(), id)
		if !report(w, err) {
			network.WriteJSON(w, 201, map[string]any{"token": token, "test_only": true, "qualification": "declared_test_capacity_not_hardware_proof"})
		}
	})
	m.HandleFunc("POST /commonbeam/v1/fund", func(w http.ResponseWriter, r *http.Request) {
		if !s.auth(w, r, s.Sponsor) {
			return
		}
		var in struct {
			Pool  string `json:"pool"`
			Units int64  `json:"units"`
		}
		if network.ReadJSON(w, r, &in) && !report(w, s.Store.Fund(r.Context(), in.Pool, in.Units)) {
			network.WriteJSON(w, 200, map[string]any{"test_only": true, "funded_units": in.Units})
		}
	})
	m.HandleFunc("POST /commonbeam/v1/reallocate", func(w http.ResponseWriter, r *http.Request) {
		if !s.auth(w, r, s.Sponsor) {
			return
		}
		var in struct {
			From  string `json:"from"`
			To    string `json:"to"`
			Units int64  `json:"units"`
		}
		if network.ReadJSON(w, r, &in) && !report(w, s.Store.Reallocate(r.Context(), in.From, in.To, in.Units)) {
			network.WriteJSON(w, 200, map[string]bool{"test_only": true, "unallocated_only": true})
		}
	})
	m.HandleFunc("POST /commonbeam/v1/revoke/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !s.auth(w, r, s.Admin) {
			return
		}
		if !report(w, s.Store.Revoke(r.Context(), r.PathValue("id"))) {
			network.WriteJSON(w, 200, map[string]bool{"revoked": true})
		}
	})
	m.HandleFunc("POST /commonbeam/v1/grant", func(w http.ResponseWriter, r *http.Request) {
		if !s.auth(w, r, s.Admin) {
			return
		}
		var in struct {
			Pool  string `json:"pool"`
			Units int64  `json:"units"`
		}
		if !network.ReadJSON(w, r, &in) {
			return
		}
		token, err := s.Store.Grant(r.Context(), in.Pool, in.Units)
		if !report(w, err) {
			network.WriteJSON(w, 201, map[string]any{"token": token, "test_only": true})
		}
	})
	m.HandleFunc("POST /commonbeam/v1/session", func(w http.ResponseWriter, r *http.Request) {
		if !s.auth(w, r, s.Router) {
			return
		}
		token, err := s.Store.Grant(r.Context(), "public", 4096)
		if !report(w, err) {
			network.WriteJSON(w, 201, map[string]any{"token": token, "test_only": true,
				"identity": "pseudonymous_session_not_unique_person", "units": 4096})
		}
	})
	m.HandleFunc("GET /commonbeam/v1/balance", func(w http.ResponseWriter, r *http.Request) {
		if !s.auth(w, r, s.Router) {
			return
		}
		balance, err := s.Store.Balance(r.Context(), r.Header.Get("X-Commonbeam-Account"))
		if !report(w, err) {
			network.WriteJSON(w, 200, map[string]any{"balance": balance, "test_only": true})
		}
	})
	m.HandleFunc("POST /commonbeam/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
		if !s.auth(w, r, s.Router) {
			return
		}
		var in struct {
			Request     protocol.ChatRequest `json:"request"`
			Idempotency string               `json:"idempotency"`
		}
		if !network.ReadJSON(w, r, &in) {
			return
		}
		j, err := s.Store.Reserve(r.Context(), r.Header.Get("X-Commonbeam-Account"), in.Idempotency, in.Request)
		if !report(w, err) {
			j.Request = nil
			network.WriteJSON(w, 201, j)
		}
	})
	m.HandleFunc("GET /commonbeam/v1/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !s.auth(w, r, s.Router) {
			return
		}
		j, err := s.Store.Job(r.Context(), r.PathValue("id"), r.Header.Get("X-Commonbeam-Account"), "", false)
		if !report(w, err) {
			network.WriteJSON(w, 200, j)
		}
	})
	m.HandleFunc("POST /commonbeam/v1/jobs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		if !s.auth(w, r, s.Router) {
			return
		}
		if !report(w, s.Store.Cancel(r.Context(), r.Header.Get("X-Commonbeam-Account"), r.PathValue("id"))) {
			network.WriteJSON(w, 200, map[string]bool{"cancellation_requested": true})
		}
	})
	m.HandleFunc("GET /commonbeam/v1/work", func(w http.ResponseWriter, r *http.Request) {
		id := s.provider(w, r)
		if id == "" {
			return
		}
		j, err := s.Store.Claim(r.Context(), id)
		if !report(w, err) {
			network.WriteJSON(w, 200, map[string]any{"job": j})
		}
	})
	m.HandleFunc("POST /commonbeam/v1/work/{id}/append", func(w http.ResponseWriter, r *http.Request) {
		id := s.provider(w, r)
		if id == "" {
			return
		}
		var in struct {
			Text string `json:"text"`
		}
		if network.ReadJSON(w, r, &in) && !report(w, s.Store.Append(r.Context(), id, r.PathValue("id"), in.Text)) {
			network.WriteJSON(w, 200, map[string]bool{"ok": true})
		}
	})
	m.HandleFunc("POST /commonbeam/v1/work/{id}/finish", func(w http.ResponseWriter, r *http.Request) {
		id := s.provider(w, r)
		if id == "" {
			return
		}
		var in struct {
			Receipt *protocol.SignedReceipt `json:"receipt"`
		}
		if network.ReadJSON(w, r, &in) && !report(w, s.Store.Finish(r.Context(), id, r.PathValue("id"), in.Receipt)) {
			network.WriteJSON(w, 200, map[string]bool{"ok": true})
		}
	})
	m.HandleFunc("GET /commonbeam/v1/audit/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !s.auth(w, r, s.Admin) {
			return
		}
		out, err := s.Store.Audit(r.Context(), r.PathValue("id"))
		if !report(w, err) {
			network.WriteJSON(w, 200, out)
		}
	})
	return network.Pilot(m)
}

func (s *Server) Reconcile(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c, cancel := context.WithTimeout(ctx, 5*time.Second)
			s.Store.Reap(c)
			cancel()
		}
	}
}
