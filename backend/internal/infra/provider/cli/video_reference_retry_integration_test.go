//go:build integration

package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/infra/provider"
	"github.com/chenyme/grok2api/backend/internal/infra/security"
)

// TestIntegrationVideoReferenceRetry 通过完整适配器与 HTTP 上游验证参考图重建次数和排除边界。
func TestIntegrationVideoReferenceRetry(t *testing.T) {
	cases := []struct {
		name       string
		createCode int
		createBody string
		pollBody   string
		failures   int
		references bool
		cancel     bool
		wantCalls  int
		wantOK     bool
	}{
		{name: "poll_failure_recovers", pollBody: `{"status":"failed","error":{"code":"image_reference_transfer_failed","message":"invalid image upload"}}`, failures: 1, references: true, wantCalls: 2, wantOK: true},
		{name: "three_retries_then_stop", pollBody: `{"status":"failed","error":{"code":"image_reference_transfer_failed"}}`, failures: 10, references: true, wantCalls: 4},
		{name: "create_rejection_recovers", createCode: 400, createBody: `{"error":{"code":"image_reference_transfer_failed","message":"invalid image upload"}}`, failures: 1, references: true, wantCalls: 2, wantOK: true},
		{name: "ordinary_429", createCode: 429, createBody: `{"error":{"message":"invalid image upload"}}`, failures: 10, references: true, wantCalls: 1},
		{name: "ordinary_500", createCode: 500, createBody: `{"error":{"message":"internal server error"}}`, failures: 10, references: true, wantCalls: 1},
		{name: "no_reference", pollBody: `{"status":"failed","error":{"message":"invalid image upload"}}`, failures: 10, wantCalls: 1},
		{name: "running_not_recreated", pollBody: `{"status":"running","error":{"message":"invalid image upload"}}`, failures: 10, references: true, wantCalls: 1},
		{name: "cancelled_not_recreated", pollBody: `{"status":"cancelled","error":{"message":"invalid image upload"}}`, failures: 10, references: true, wantCalls: 1},
		{name: "creation_with_id_not_recreated", createBody: `{"request_id":"accepted","error":{"message":"invalid image upload"}}`, failures: 10, references: true, wantCalls: 1},
		{name: "cancel_backoff", pollBody: `{"status":"failed","error":{"message":"invalid image upload"}}`, failures: 10, references: true, cancel: true, wantCalls: 1},
		{name: "policy_not_recreated", pollBody: `{"status":"failed","error":{"message":"content violates usage guidelines: invalid image upload"}}`, failures: 10, references: true, wantCalls: 1},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			cipher, err := security.NewCipher(base64.StdEncoding.EncodeToString(make([]byte, 32)))
			if err != nil {
				t.Fatal(err)
			}
			token, err := cipher.Encrypt("integration-only")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			var mu sync.Mutex
			calls := 0
			var payloads []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if r.Header.Get("Authorization") != "Bearer integration-only" {
					t.Error("credential changed during retry")
				}
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/v1/videos/generations":
					calls++
					raw, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					var decoded any
					if json.Unmarshal(raw, &decoded) != nil {
						t.Error("invalid create JSON")
					}
					payloads = append(payloads, string(raw))
					if calls <= test.failures && test.createBody != "" {
						if test.createCode != 0 {
							w.WriteHeader(test.createCode)
						}
						_, _ = io.WriteString(w, test.createBody)
						return
					}
					_, _ = io.WriteString(w, `{"request_id":"job-`+string(rune('0'+calls))+`"}`)
				case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/videos/job-"):
					if calls <= test.failures && test.pollBody != "" {
						_, _ = io.WriteString(w, test.pollBody)
						if test.cancel {
							cancel()
						}
						return
					}
					_, _ = io.WriteString(w, `{"status":"completed","video":{"url":"https://assets.grok.com/final.mp4"}}`)
				default:
					t.Errorf("unexpected HTTP call: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			adapter := NewAdapter(Config{BaseURL: server.URL + "/v1"}, cipher)
			input := provider.VideoRequest{
				Credential: account.Credential{
					ID: 1, Provider: account.ProviderBuild, BuildRouteMode: account.BuildRouteBuild,
					EncryptedAccessToken: token,
				},
				JobID: "integration", Model: buildVideoModel, Prompt: "animate", Duration: 6,
			}
			if test.references {
				input.ReferenceURLs = []string{"https://example.com/reference.png"}
			}
			start := time.Now()
			result, err := adapter.GenerateVideo(ctx, input)
			if (err == nil) != test.wantOK {
				t.Fatalf("result=%+v error=%v", result, err)
			}
			if test.cancel && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled request returned %v", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if calls != test.wantCalls {
				t.Fatalf("create calls=%d want=%d", calls, test.wantCalls)
			}
			for _, payload := range payloads {
				if payload != payloads[0] {
					t.Fatal("reference URL or generation parameters changed")
				}
			}
			if test.wantCalls == 4 && time.Since(start) < 14*time.Second {
				t.Fatal("2/4/8 second backoff was skipped")
			}
		})
	}
}
