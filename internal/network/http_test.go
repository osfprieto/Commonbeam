package network

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrowserCannotConsumeLocalPublicPool(t *testing.T) {
	called := 0
	handler := Pilot(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(201)
	}))
	for _, header := range []string{"Origin", "Sec-Fetch-Site"} {
		req := httptest.NewRequest("POST", "/commonbeam/v1/session", nil)
		value := "https://untrusted.invalid"
		if header == "Sec-Fetch-Site" {
			value = "cross-site"
		}
		req.Header.Set(header, value)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != 403 || called != 0 {
			t.Fatal("browser request reached allocation logic")
		}
	}
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/commonbeam/v1/session", nil))
	if called != 1 {
		t.Fatal("ordinary API/CLI client was blocked")
	}
}
