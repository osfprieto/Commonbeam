package inference

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/osfprieto/Commonbeam/internal/protocol"
)

type Backend interface {
	Generate(context.Context, protocol.ChatRequest, func(string) error) (protocol.Usage, error)
}

type Fake struct{}

func (Fake) Generate(ctx context.Context, r protocol.ChatRequest, emit func(string) error) (protocol.Usage, error) {
	words := strings.Fields(protocol.FakeText)
	if r.MaxTokens < len(words) {
		words = words[:r.MaxTokens]
	}
	for i, word := range words {
		if err := ctx.Err(); err != nil {
			return protocol.Usage{}, err
		}
		if i > 0 {
			word = " " + word
		}
		if err := emit(word); err != nil {
			return protocol.Usage{}, err
		}
	}
	n := len(words)
	return protocol.Usage{PromptTokens: r.InputBudget(), CompletionTokens: n, TotalTokens: r.InputBudget() + n}, nil
}

type HTTP struct {
	BaseURL string
	APIKey  string
	Client  *http.Client
}

// The approved pilot permits only loopback model servers; no peer-provided URL is dialed.
func NewHTTP(base, key string) (*HTTP, error) {
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("backend must be an HTTP loopback URL without credentials or query")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return nil, errors.New("backend URL must use a loopback IP literal")
	}
	return &HTTP{strings.TrimRight(base, "/"), key, &http.Client{
		Timeout:       110 * time.Second,
		Transport:     &http.Transport{Proxy: nil},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (b *HTTP) Probe(ctx context.Context, model string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.BaseURL+"/models", nil)
	if err != nil {
		return err
	}
	if b.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+b.APIKey)
	}
	resp, err := b.Client.Do(req)
	if err != nil {
		return errors.New("model server unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("model probe returned HTTP %d", resp.StatusCode)
	}
	var models struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&models); err != nil {
		return errors.New("invalid models response")
	}
	for _, m := range models.Data {
		if m.ID == model {
			return nil
		}
	}
	return errors.New("declared model not advertised by server")
}

func (b *HTTP) Generate(ctx context.Context, r protocol.ChatRequest, emit func(string) error) (protocol.Usage, error) {
	if err := r.Validate(); err != nil {
		return protocol.Usage{}, err
	}
	payload := struct {
		protocol.ChatRequest
		StreamOptions struct {
			IncludeUsage bool `json:"include_usage"`
		} `json:"stream_options"`
	}{ChatRequest: r}
	payload.Stream = true
	payload.StreamOptions.IncludeUsage = true
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return protocol.Usage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if b.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+b.APIKey)
	}
	resp, err := b.Client.Do(req)
	if err != nil {
		return protocol.Usage{}, errors.New("model request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return protocol.Usage{}, fmt.Errorf("model request returned HTTP %d", resp.StatusCode)
	}
	scanner := bufio.NewScanner(io.LimitReader(resp.Body, 2<<20))
	scanner.Buffer(make([]byte, 4096), 128<<10)
	var usage *protocol.Usage
	done := false
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			done = true
			break
		}
		var event struct {
			Model   string `json:"model"`
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *protocol.Usage `json:"usage"`
			Error json.RawMessage `json:"error"`
		}
		if json.Unmarshal([]byte(data), &event) != nil || len(event.Error) > 0 && string(event.Error) != "null" {
			return protocol.Usage{}, errors.New("invalid upstream stream event")
		}
		if event.Model != "" && event.Model != r.Model {
			return protocol.Usage{}, errors.New("upstream changed model")
		}
		if len(event.Choices) > 1 {
			return protocol.Usage{}, errors.New("multiple choices are unsupported")
		}
		for _, c := range event.Choices {
			if err := emit(c.Delta.Content); err != nil {
				return protocol.Usage{}, err
			}
		}
		if event.Usage != nil {
			usage = event.Usage
		}
	}
	if scanner.Err() != nil || !done || usage == nil {
		return protocol.Usage{}, errors.New("incomplete upstream stream or missing usage")
	}
	if usage.PromptTokens < 0 || usage.CompletionTokens < 0 || usage.CompletionTokens > r.MaxTokens ||
		usage.TotalTokens != usage.PromptTokens+usage.CompletionTokens || usage.TotalTokens > r.Budget() {
		return protocol.Usage{}, errors.New("upstream usage outside reservation")
	}
	return *usage, nil
}
