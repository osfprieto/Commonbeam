package protocol

import "testing"

func FuzzAdmissionBounds(f *testing.F) {
	f.Add("hello", 512)
	f.Add("long context", -1)
	f.Fuzz(func(t *testing.T, text string, limit int) {
		if len(text) > 65536 {
			return
		}
		r := ChatRequest{Model: FakeModel, Messages: []Message{{Role: "user", Content: text}}, MaxTokens: limit}
		if err := r.Validate(); err == nil {
			if r.MaxTokens < 1 || r.MaxTokens > 512 || r.Budget() > 4096 {
				t.Fatal("valid request escaped admission bounds")
			}
		}
	})
}
