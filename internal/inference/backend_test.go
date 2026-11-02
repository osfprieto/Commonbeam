package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/osfprieto/Commonbeam/internal/protocol"
)

func TestAdapterConformance(t *testing.T) {
	for _, tc := range []struct {
		name, stream string
		wantError    bool
	}{
		{"complete", "data: {\"model\":\"test\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\ndata: [DONE]\n\n", false},
		{"missing_usage", "data: [DONE]\n\n", true},
		{"truncated", "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\n", true},
		{"wrong_model", "data: {\"model\":\"other\",\"choices\":[]}\n\ndata: [DONE]\n\n", true},
		{"overspend", "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":20000,\"completion_tokens\":1,\"total_tokens\":20001}}\n\ndata: [DONE]\n\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/models" {
					fmt.Fprint(w, `{"data":[{"id":"test"}]}`)
					return
				}
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if value, present := request["temperature"]; !present || value != float64(0) {
					t.Error("backend default temperature leaked into the approved profile")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, tc.stream)
			}))
			defer server.Close()
			b, err := NewHTTP(server.URL+"/v1", "")
			if err != nil {
				t.Fatal(err)
			}
			if err := b.Probe(context.Background(), "test"); err != nil {
				t.Fatal(err)
			}
			var text strings.Builder
			_, err = b.Generate(context.Background(), protocol.ChatRequest{Model: "test", MaxTokens: 10, Messages: []protocol.Message{{Role: "user", Content: "test"}}},
				func(s string) error { text.WriteString(s); return nil })
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v; wantError=%v", err, tc.wantError)
			}
			if !tc.wantError && text.String() != "hello" {
				t.Fatal("stream content lost")
			}
		})
	}
}

func TestBackendRejectsUntrustedTargets(t *testing.T) {
	for _, target := range []string{"http://169.254.169.254/v1", "http://localhost/v1", "http://user:secret@127.0.0.1/v1", "http://127.0.0.1/v1?secret=x"} {
		if _, err := NewHTTP(target, ""); err == nil {
			t.Fatalf("unsafe backend URL accepted: %s", target)
		}
	}
}

func TestFakeBudgetAndCancellation(t *testing.T) {
	r := protocol.ChatRequest{MaxTokens: 2, Messages: []protocol.Message{{Role: "user", Content: "test"}}}
	var result string
	u, err := (Fake{}).Generate(context.Background(), r, func(s string) error { result += s; return nil })
	if err != nil || result != "Commonbeam simulation." || u.CompletionTokens != 2 {
		t.Fatalf("unexpected simulation result: %v %v", u, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (Fake{}).Generate(ctx, r, func(string) error { t.Fatal("emitted after cancellation"); return nil }); err == nil {
		t.Fatal("cancelled generation succeeded")
	}
}
