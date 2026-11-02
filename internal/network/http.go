package network

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.Status, e.Message) }

type Client struct {
	Base  string
	Token string
	HTTP  *http.Client
}

func NewClient(base, token string) *Client {
	return &Client{strings.TrimRight(base, "/"), token, &http.Client{
		Timeout:       5 * time.Second,
		Transport:     &http.Transport{Proxy: nil},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (c *Client) Call(ctx context.Context, method, path, account string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, body)
	if err != nil {
		return errors.New("invalid endpoint")
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if account != "" {
		req.Header.Set("X-Commonbeam-Account", account)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return errors.New("network endpoint unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return &APIError{resp.StatusCode, http.StatusText(resp.StatusCode)}
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(out)
	}
	return nil
}

func Bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(h, "Bearer ")
}

func Authorized(r *http.Request, token string) bool {
	return token != "" && subtle.ConstantTimeCompare([]byte(Bearer(r)), []byte(token)) == 1
}

func ReadJSON(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		WriteError(w, 400, "invalid or unsupported JSON request", "invalid_request_error")
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		WriteError(w, 400, "request must contain one JSON value", "invalid_request_error")
		return false
	}
	return true
}

func WriteJSON(w http.ResponseWriter, status int, out any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(out)
}

func WriteError(w http.ResponseWriter, status int, message, code string) {
	WriteJSON(w, status, map[string]any{"error": map[string]any{
		"message": message, "type": code, "code": code, "param": nil,
	}})
}

// Browser clients are deferred; prevent drive-by requests from draining local test pools.
func Pilot(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			WriteError(w, 403, "browser origins are not enabled in the local pilot", "origin_not_allowed")
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func Serve(ctx context.Context, addr string, handler http.Handler, container bool) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return errors.New("listen address must include an IP and port")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() && !container {
		return errors.New("pilot binds only to loopback; container networking requires explicit opt-in")
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 130 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			server.Shutdown(c)
		case <-done:
		}
	}()
	err = server.Serve(listener)
	close(done)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
