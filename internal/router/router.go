package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/osfprieto/Commonbeam/internal/network"
	"github.com/osfprieto/Commonbeam/internal/protocol"
	"github.com/osfprieto/Commonbeam/internal/store"
)

type Server struct{ Authority *network.Client }

func upstream(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	var api *network.APIError
	if errors.As(err, &api) {
		network.WriteError(w, api.Status, api.Message, "upstream_error")
	} else {
		network.WriteError(w, 503, "accounting authority unavailable", "authority_unavailable")
	}
	return true
}

func (s *Server) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		var out any
		if !upstream(w, s.Authority.Call(r.Context(), "GET", "/healthz", "", nil, &out)) {
			network.WriteJSON(w, 200, out)
		}
	})
	m.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		var providers []store.Provider
		if upstream(w, s.Authority.Call(r.Context(), "GET", "/commonbeam/v1/providers", "", nil, &providers)) {
			return
		}
		data := []any{}
		seen := map[string]bool{}
		for _, p := range providers {
			id := p.Signed.Manifest.Model
			if p.Active && !seen[id] {
				data = append(data, map[string]any{"id": id, "object": "model", "created": 0, "owned_by": "commonbeam-test"})
				seen[id] = true
			}
		}
		network.WriteJSON(w, 200, map[string]any{"object": "list", "data": data})
	})
	m.HandleFunc("GET /commonbeam/v1/status", func(w http.ResponseWriter, r *http.Request) {
		var out any
		if !upstream(w, s.Authority.Call(r.Context(), "GET", "/commonbeam/v1/status", "", nil, &out)) {
			network.WriteJSON(w, 200, out)
		}
	})
	m.HandleFunc("POST /commonbeam/v1/session", func(w http.ResponseWriter, r *http.Request) {
		var out any
		if !upstream(w, s.Authority.Call(r.Context(), "POST", "/commonbeam/v1/session", "", nil, &out)) {
			network.WriteJSON(w, 201, out)
		}
	})
	m.HandleFunc("GET /commonbeam/v1/balance", func(w http.ResponseWriter, r *http.Request) {
		var out any
		if !upstream(w, s.Authority.Call(r.Context(), "GET", "/commonbeam/v1/balance", network.Bearer(r), nil, &out)) {
			network.WriteJSON(w, 200, out)
		}
	})
	m.HandleFunc("GET /commonbeam/v1/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		var out any
		if !upstream(w, s.Authority.Call(r.Context(), "GET", "/commonbeam/v1/jobs/"+r.PathValue("id"), network.Bearer(r), nil, &out)) {
			network.WriteJSON(w, 200, out)
		}
	})
	m.HandleFunc("POST /commonbeam/v1/jobs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		if !upstream(w, s.Authority.Call(r.Context(), "POST", "/commonbeam/v1/jobs/"+r.PathValue("id")+"/cancel", network.Bearer(r), nil, nil)) {
			network.WriteJSON(w, 200, map[string]bool{"cancellation_requested": true})
		}
	})
	m.HandleFunc("POST /v1/chat/completions", s.chat)
	return network.Pilot(m)
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	account := network.Bearer(r)
	if account == "" {
		network.WriteError(w, 401, "create a bounded public session first", "authentication_error")
		return
	}
	var request protocol.ChatRequest
	if !network.ReadJSON(w, r, &request) {
		return
	}
	if err := request.Validate(); err != nil {
		network.WriteError(w, 400, err.Error(), "invalid_request_error")
		return
	}
	idem := r.Header.Get("Idempotency-Key")
	if idem == "" {
		idem = protocol.RandomID()
	}
	var j protocol.Job
	in := map[string]any{"request": request, "idempotency": idem}
	if upstream(w, s.Authority.Call(r.Context(), "POST", "/commonbeam/v1/jobs", account, in, &j)) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 125*time.Second)
	defer cancel()
	defer func() {
		if !protocol.Terminal(j.State) {
			c, done := context.WithTimeout(context.Background(), 3*time.Second)
			defer done()
			s.Authority.Call(c, "POST", "/commonbeam/v1/jobs/"+j.ID+"/cancel", account, nil, nil)
		}
	}()
	w.Header().Set("X-Commonbeam-Job-Id", j.ID)
	w.Header().Set("Idempotency-Key", idem)
	w.Header().Set("Cache-Control", "no-store")
	offset := 0
	if request.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Accel-Buffering", "no")
		if !event(w, chunk(j.ID, request.Model, map[string]string{"role": "assistant"}, nil, nil)) {
			return
		}
	}
	t := time.NewTicker(40 * time.Millisecond)
	defer t.Stop()
	for {
		if request.Stream && len(j.Output) > offset {
			if !event(w, chunk(j.ID, request.Model, map[string]string{"content": j.Output[offset:]}, nil, nil)) {
				return
			}
			offset = len(j.Output)
		}
		if protocol.Terminal(j.State) {
			if j.State != "complete" || j.Receipt == nil {
				if request.Stream {
					event(w, map[string]any{"error": map[string]string{"message": "execution did not complete", "type": j.State}})
					done(w)
				} else {
					network.WriteError(w, 503, "execution did not complete", j.State)
				}
				return
			}
			finishReason := "stop"
			if j.Receipt.Receipt.Usage.CompletionTokens >= request.MaxTokens {
				finishReason = "length"
			}
			if request.Stream {
				event(w, chunk(j.ID, request.Model, map[string]string{}, finishReason, j.Receipt.Receipt.Usage))
				done(w)
			} else {
				network.WriteJSON(w, 200, map[string]any{"id": "chatcmpl-" + j.ID, "object": "chat.completion",
					"created": time.Now().Unix(), "model": request.Model,
					"choices": []any{map[string]any{"index": 0, "message": map[string]string{"role": "assistant", "content": j.Output}, "finish_reason": finishReason}},
					"usage":   j.Receipt.Receipt.Usage})
			}
			return
		}
		select {
		case <-ctx.Done():
			if request.Stream {
				event(w, map[string]any{"error": map[string]string{"message": "execution interrupted; query job status before retrying", "type": "interrupted"}})
				done(w)
			} else {
				network.WriteError(w, 504, "execution interrupted; query job status", "interrupted")
			}
			return
		case <-t.C:
		}
		if err := s.Authority.Call(ctx, "GET", "/commonbeam/v1/jobs/"+j.ID, account, nil, &j); err != nil {
			if request.Stream {
				event(w, map[string]any{"error": map[string]string{"message": "authority unavailable; execution state is ambiguous", "type": "authority_unavailable"}})
				done(w)
			} else {
				upstream(w, err)
			}
			return
		}
	}
}

func chunk(id, model string, delta any, finish, usage any) map[string]any {
	out := map[string]any{"id": "chatcmpl-" + id, "object": "chat.completion.chunk", "created": time.Now().Unix(),
		"model": model, "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}}
	if usage != nil {
		out["usage"] = usage
	}
	return out
}

func event(w http.ResponseWriter, out any) bool {
	b, _ := json.Marshal(out)
	_, err := fmt.Fprintf(w, "data: %s\n\n", b)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return err == nil
}

func done(w http.ResponseWriter) {
	fmt.Fprint(w, "data: [DONE]\n\n")
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// Model IDs are not filesystem paths; the API never resolves them as local files.
func ValidModelID(id string) bool { return id != "" && !strings.ContainsAny(id, "\r\n") }
