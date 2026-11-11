//go:build integration

package inference

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	accountapp "github.com/chenyme/grok2api/backend/internal/application/account"
	clientkeyapp "github.com/chenyme/grok2api/backend/internal/application/clientkey"
	"github.com/chenyme/grok2api/backend/internal/application/gateway"
	"github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/domain/clientkey"
	"github.com/chenyme/grok2api/backend/internal/domain/media"
	"github.com/chenyme/grok2api/backend/internal/infra/persistence/relational"
	"github.com/chenyme/grok2api/backend/internal/infra/provider"
	build "github.com/chenyme/grok2api/backend/internal/infra/provider/cli"
	"github.com/chenyme/grok2api/backend/internal/infra/runtime/memory"
	"github.com/chenyme/grok2api/backend/internal/infra/security"
	"github.com/chenyme/grok2api/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

// TestIntegrationVideoContentPolicy 验证本地 HTTP、真实 Build 适配器、持久终态及公开查询，不调用外部供应商。
func TestIntegrationVideoContentPolicy(t *testing.T) {
	for _, scenario := range []struct {
		name, createBody, pollBody string
	}{
		{"create_code_only", `{"error":{"code":"CONTENT_POLICY_VIOLATION"}}`, ""},
		{"poll_code_only", `{"request_id":"fixture"}`, `{"data":{"status":"failed","error":{"code":"CONTENT_POLICY_VIOLATION"}}}`},
		{"poll_message", `{"request_id":"fixture"}`, `{"status":"failed","error":{"message":"CONTENT_POLICY_VIOLATION"}}`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			db, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "integration.db"))
			must(err)
			defer db.Close()
			must(db.InitializeSchema(ctx))
			cipher, err := security.NewCipher(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
			must(err)
			var creates, polls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/videos/generations"):
					creates.Add(1)
					_, _ = io.WriteString(w, scenario.createBody)
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/videos/fixture"):
					polls.Add(1)
					_, _ = io.WriteString(w, scenario.pollBody)
				default:
					http.Error(w, "unexpected integration request", http.StatusNotFound)
				}
			}))
			defer upstream.Close()
			accounts, models := relational.NewAccountRepository(db), relational.NewModelRepository(db)
			audits, jobs := relational.NewAuditRepository(db), relational.NewMediaJobRepository(db)
			keys := relational.NewClientKeyRepository(db)
			key, err := keys.Create(ctx, clientkey.Key{Name: "policy-integration", Prefix: "fixture", SecretHash: strings.Repeat("a", 64),
				EncryptedSecret: "unused", Enabled: true, RPMLimit: 60, MaxConcurrent: 4})
			must(err)
			const model = "grok-imagine-video-1.5"
			must(models.UpsertDiscovered(ctx, account.ProviderBuild, []string{model}))
			for index := range 2 {
				token, err := cipher.Encrypt("fixture-token")
				must(err)
				credential, _, err := accounts.UpsertByIdentity(ctx, account.Credential{
					Provider: account.ProviderBuild, AuthType: account.AuthTypeOAuth, BuildRouteMode: account.BuildRouteBuild,
					Name: fmt.Sprintf("fixture-%d", index), SourceKey: fmt.Sprintf("fixture-%d", index),
					EncryptedAccessToken: token, ExpiresAt: time.Now().Add(time.Hour), Enabled: true,
					AuthStatus: account.AuthStatusActive, Priority: 100 - index, MaxConcurrent: 1,
				})
				must(err)
				must(models.ReplaceAccountCapabilities(ctx, credential.ID, []string{model}, time.Now()))
			}
			registry := provider.NewRegistry(build.NewAdapter(build.Config{BaseURL: upstream.URL, FallbackBaseURL: upstream.URL}, cipher))
			sticky := memory.NewStickyStore()
			accountService := accountapp.NewService(accounts, audits, memory.NewDeviceSessionStore(), sticky, registry, cipher, nil)
			selector := gateway.NewSelector(accounts, memory.NewConcurrencyLimiter(), sticky, registry, time.Hour, time.Second, time.Minute)
			service := gateway.NewService(models, audits, accountService, clientkeyapp.NewService(keys, nil, nil, 60, 4, nil), registry, selector, nil, 3)
			service.ConfigureMedia(jobs, 1)
			service.UpdateVideoMaxAttempts(3)
			done := make(chan struct{})
			go func() { service.RunVideoWorkers(ctx); close(done) }()
			defer func() { cancel(); <-done }()
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(middleware.ClientKey, key)
				c.Set(middleware.RequestIDKey, scenario.name)
				c.Next()
			})
			NewHandler(service, nil, 1<<20).Register(router.Group("/v1"))
			api := httptest.NewServer(router)
			defer api.Close()
			response, err := http.Post(api.URL+"/v1/videos", "application/json", strings.NewReader(`{"model":"Build/grok-imagine-video-1.5","prompt":"integration fixture","seconds":"6","resolution":"720p"}`))
			must(err)
			var created struct {
				ID string `json:"id"`
			}
			must(json.NewDecoder(response.Body).Decode(&created))
			response.Body.Close()
			if response.StatusCode >= 400 || created.ID == "" {
				t.Fatalf("create status=%d id=%q", response.StatusCode, created.ID)
			}
			deadline := time.Now().Add(15 * time.Second)
			for {
				job, err := jobs.GetMediaJob(ctx, created.ID, key.ID)
				must(err)
				if job.Status == media.StatusFailed {
					if job.ErrorCode != provider.ContentPolicyCode {
						t.Fatalf("persisted error code=%s", job.ErrorCode)
					}
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("video terminal deadline")
				}
				time.Sleep(10 * time.Millisecond)
			}
			response, err = http.Get(api.URL + "/v1/videos/" + created.ID)
			must(err)
			var result struct {
				Error struct{ Code, Message string }
			}
			must(json.NewDecoder(response.Body).Decode(&result))
			response.Body.Close()
			if response.StatusCode != 200 || result.Error.Code != provider.ContentPolicyCode || result.Error.Message != provider.ContentPolicyMessage {
				t.Fatalf("public status=%d error=%+v", response.StatusCode, result.Error)
			}
			if creates.Load() != 1 || scenario.pollBody != "" && polls.Load() != 1 {
				t.Fatalf("policy rejection retried: creates=%d polls=%d", creates.Load(), polls.Load())
			}
		})
	}
}
