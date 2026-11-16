//go:build integration

package inference_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	accountapp "github.com/chenyme/grok2api/backend/internal/application/account"
	clientkeyapp "github.com/chenyme/grok2api/backend/internal/application/clientkey"
	"github.com/chenyme/grok2api/backend/internal/application/gateway"
	mediaapp "github.com/chenyme/grok2api/backend/internal/application/media"
	"github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/domain/clientkey"
	"github.com/chenyme/grok2api/backend/internal/domain/media"
	"github.com/chenyme/grok2api/backend/internal/domain/model"
	infraegress "github.com/chenyme/grok2api/backend/internal/infra/egress"
	localmedia "github.com/chenyme/grok2api/backend/internal/infra/media"
	"github.com/chenyme/grok2api/backend/internal/infra/persistence/relational"
	"github.com/chenyme/grok2api/backend/internal/infra/provider"
	build "github.com/chenyme/grok2api/backend/internal/infra/provider/cli"
	"github.com/chenyme/grok2api/backend/internal/infra/provider/console"
	"github.com/chenyme/grok2api/backend/internal/infra/runtime/memory"
	"github.com/chenyme/grok2api/backend/internal/infra/security"
	"github.com/chenyme/grok2api/backend/internal/transport/http/inference"
	mediahttp "github.com/chenyme/grok2api/backend/internal/transport/http/media"
	"github.com/chenyme/grok2api/backend/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
)

// TestIntegrationBuildVideoOperations 经真实 HTTP 鉴权、SQLite 选号、Build 适配器和上传票据验证编辑及延长闭环。
func TestIntegrationBuildVideoOperations(t *testing.T) {
	for _, scenario := range []struct {
		operation provider.VideoOperation
		duration  int
		mode      account.BuildRouteMode
	}{
		{provider.VideoOperationExtend, 2, account.BuildRouteAuto},
		{provider.VideoOperationExtend, 6, account.BuildRouteBuild},
		{provider.VideoOperationExtend, 10, account.BuildRouteXAI},
		{provider.VideoOperationEdit, 0, account.BuildRouteAuto},
		{provider.VideoOperationEdit, 0, account.BuildRouteBuild},
		{provider.VideoOperationEdit, 0, account.BuildRouteXAI},
	} {
		t.Run(fmt.Sprintf("%s_%ds_%s", scenario.operation, scenario.duration, scenario.mode), func(t *testing.T) {
			endpoint := "/v1/videos/extensions"
			if scenario.operation == provider.VideoOperationEdit {
				endpoint = "/v1/videos/edits"
			}
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			db, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "extension.db"))
			must(err)
			defer db.Close()
			must(db.InitializeSchema(ctx))
			cipher, err := security.NewCipher(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32)))
			must(err)
			accounts, models := relational.NewAccountRepository(db), relational.NewModelRepository(db)
			audits, jobs := relational.NewAuditRepository(db), relational.NewMediaJobRepository(db)
			keys := relational.NewClientKeyRepository(db)
			var buildCredential account.Credential
			var buildRoute model.Route
			for _, p := range []account.Provider{account.ProviderConsole, account.ProviderBuild} {
				encrypted, err := cipher.Encrypt(string(p) + "-fixture")
				must(err)
				credential, _, err := accounts.UpsertByIdentity(ctx, account.Credential{
					Provider: p, AuthType: account.AuthTypeOAuth, Name: string(p), SourceKey: string(p),
					EncryptedAccessToken: encrypted, ExpiresAt: time.Now().Add(time.Hour), BuildRouteMode: scenario.mode,
					Enabled: true, AuthStatus: account.AuthStatusActive, Priority: 100, MaxConcurrent: 1,
				})
				must(err)
				upstreamModel := "grok-imagine-video"
				if p == account.ProviderBuild {
					upstreamModel = "grok-imagine-video-1.5"
					buildCredential = credential
				}
				route, err := models.Create(ctx, model.Route{
					PublicID: p.ModelNamespace() + "/extension-fixture", Provider: p, UpstreamModel: upstreamModel,
					Capability: model.CapabilityVideo, Origin: model.OriginManual, Enabled: true,
				}, []uint64{credential.ID})
				must(err)
				if p == account.ProviderBuild {
					buildRoute = route
				}
			}
			objects, err := localmedia.NewLocalStore(filepath.Join(t.TempDir(), "objects"))
			must(err)
			mediaService := mediaapp.NewServiceWithTickets(
				relational.NewMediaAssetRepository(db), jobs, relational.NewMediaUploadTicketRepository(db), objects, nil,
				mediaapp.Config{PublicBaseURL: "https://media.fixture", MaxTotalBytes: 1 << 20},
			)
			mp4 := append([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}, bytes.Repeat([]byte{1}, 64)...)
			var apiURL string
			var creates, polls, otherCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/xai"+endpoint:
					creates.Add(1)
					if r.Header.Get("Authorization") != "Bearer grok_build-fixture" ||
						r.Header.Get("User-Agent") != "xai-grok-build/integration" ||
						r.Header.Get("x-grok-model-override") != "grok-imagine-video" {
						t.Error("extension did not use Build OAuth and official model headers")
					}
					var body struct {
						Model, Prompt string
						Duration      *int
						Video         struct{ URL string }
						Output        struct {
							UploadURL string `json:"upload_url"`
						}
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					if body.Model != "grok-imagine-video" || body.Prompt != "Continue smoothly" ||
						body.Video.URL != "https://source.fixture/source.mp4" {
						t.Error("extension model, duration or source changed")
					}
					if scenario.operation == provider.VideoOperationEdit {
						if body.Duration != nil {
							t.Error("edit must omit duration")
						}
					} else if body.Duration == nil || *body.Duration != scenario.duration {
						t.Error("extension duration changed")
					}
					target, err := url.Parse(body.Output.UploadURL)
					if err != nil || target.Host != "media.fixture" || !strings.HasPrefix(target.Path, "/v1/media/uploads/") {
						t.Error("missing upload ticket")
						w.WriteHeader(400)
						return
					}
					// 用本地 HTTP 映射公开回调域，实际执行同一上传 handler 和一次性票据消费。
					upload, err := http.NewRequest(http.MethodPut, apiURL+target.Path, bytes.NewReader(mp4))
					if err != nil {
						t.Error(err)
						w.WriteHeader(500)
						return
					}
					upload.Header.Set("Content-Type", "video/mp4")
					response, err := http.DefaultClient.Do(upload)
					if err != nil {
						t.Error(err)
						w.WriteHeader(500)
						return
					}
					response.Body.Close()
					if response.StatusCode != http.StatusNoContent {
						t.Errorf("callback status=%d", response.StatusCode)
					}
					_, _ = io.WriteString(w, `{"request_id":"extension-upstream"}`)
				case r.Method == http.MethodGet && r.URL.Path == "/xai/v1/videos/extension-upstream":
					polls.Add(1)
					_, _ = io.WriteString(w, `{"status":"done"}`)
				default:
					otherCalls.Add(1)
					http.Error(w, "unexpected endpoint or provider", http.StatusBadRequest)
				}
			}))
			defer upstream.Close()
			adapter := build.NewAdapter(build.Config{
				BaseURL: upstream.URL + "/build/v1", FallbackBaseURL: upstream.URL + "/xai/v1", ClientVersion: "integration",
			}, cipher)
			adapter.SetVideoUploadIssuer(mediaService)
			egress := infraegress.NewManager(relational.NewEgressRepository(db), cipher)
			registry := provider.NewRegistry(adapter, console.NewAdapter(console.Config{BaseURL: upstream.URL + "/console/v1"}, egress, cipher, nil))
			sticky := memory.NewStickyStore()
			accountService := accountapp.NewService(accounts, audits, memory.NewDeviceSessionStore(), sticky, registry, cipher, nil)
			keyService := clientkeyapp.NewService(keys, memory.NewRateLimiter(), memory.NewConcurrencyLimiter(), 60, 4, cipher)
			key, err := keyService.Create(ctx, clientkeyapp.CreateInput{Name: "extension", Enabled: true, ProviderScope: clientkey.ProviderScopeAll})
			must(err)
			selector := gateway.NewSelector(accounts, memory.NewConcurrencyLimiter(), sticky, registry, time.Hour, time.Second, time.Minute)
			service := gateway.NewService(models, audits, accountService, keyService, registry, selector, nil, 3)
			service.ConfigureMedia(jobs, 1)
			service.ConfigureMediaAssets(mediaService)
			router := gin.New()
			router.Use(middleware.RequestID())
			mediahttp.NewHandler(mediaService).RegisterPublic(router)
			inference.NewHandler(service, nil, 1<<20).Register(router.Group("/v1", middleware.ClientAuth(keyService)))
			api := httptest.NewServer(router)
			defer api.Close()
			apiURL = api.URL
			done := make(chan struct{})
			go func() { service.RunVideoWorkers(ctx); close(done) }()
			defer func() { cancel(); <-done }()
			payload := map[string]any{
				"model": "extension-fixture", "prompt": "Continue smoothly",
				"video": map[string]string{"url": "https://source.fixture/source.mp4"},
			}
			if scenario.operation == provider.VideoOperationExtend {
				payload["duration"] = scenario.duration
			}
			body, err := json.Marshal(payload)
			must(err)
			if scenario.duration == 2 {
				denied, err := keyService.Create(ctx, clientkeyapp.CreateInput{
					Name: "console-scope", Enabled: true, ProviderScope: clientkey.ProviderScopeConsole,
				})
				must(err)
				for _, invalid := range []struct {
					model, secret string
					duration      int
					start         float64
					status        int
				}{
					{"Build/extension-fixture", denied.Secret, 2, 0, 503},
					{"extension-fixture", key.Secret, 1, 0, 400},
					{"extension-fixture", key.Secret, 11, 0, 400},
					{"extension-fixture", key.Secret, 6, 6, 400},
				} {
					raw, err := json.Marshal(map[string]any{
						"model": invalid.model, "prompt": "Continue smoothly", "duration": invalid.duration,
						"video": map[string]string{"url": "https://source.fixture/source.mp4"}, "video_extension_start_time": invalid.start,
					})
					must(err)
					req, err := http.NewRequest(http.MethodPost, api.URL+"/v1/videos/extensions", bytes.NewReader(raw))
					must(err)
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Authorization", "Bearer "+invalid.secret)
					res, err := http.DefaultClient.Do(req)
					must(err)
					res.Body.Close()
					if res.StatusCode != invalid.status {
						t.Fatalf("invalid extension model=%s duration=%d start=%v status=%d want=%d",
							invalid.model, invalid.duration, invalid.start, res.StatusCode, invalid.status)
					}
				}
				if creates.Load() != 0 || otherCalls.Load() != 0 {
					t.Fatal("invalid extension reached upstream")
				}
			}
			if scenario.operation == provider.VideoOperationEdit {
				for _, field := range []string{"duration", "resolution", "image"} {
					invalid := map[string]any{"model": "extension-fixture", "prompt": "Continue smoothly",
						"video": map[string]string{"url": "https://source.fixture/source.mp4"}}
					switch field {
					case "duration":
						invalid[field] = 2
					case "resolution":
						invalid[field] = "720p"
					case "image":
						invalid[field] = map[string]string{"url": "https://source.fixture/image.png"}
					}
					raw, err := json.Marshal(invalid)
					must(err)
					req, err := http.NewRequest(http.MethodPost, api.URL+endpoint, bytes.NewReader(raw))
					must(err)
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Authorization", "Bearer "+key.Secret)
					res, err := http.DefaultClient.Do(req)
					must(err)
					res.Body.Close()
					if res.StatusCode != http.StatusBadRequest {
						t.Fatalf("edit accepted unsupported %s: status=%d", field, res.StatusCode)
					}
				}
			}
			request, err := http.NewRequest(http.MethodPost, api.URL+endpoint, bytes.NewReader(body))
			must(err)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+key.Secret)
			response, err := http.DefaultClient.Do(request)
			must(err)
			var created struct {
				RequestID string `json:"request_id"`
			}
			must(json.NewDecoder(response.Body).Decode(&created))
			response.Body.Close()
			if response.StatusCode != 200 || created.RequestID == "" {
				t.Fatalf("create status=%d task=%q", response.StatusCode, created.RequestID)
			}
			var job media.Job
			deadline := time.Now().Add(15 * time.Second)
			for {
				job, err = jobs.GetMediaJob(ctx, created.RequestID, key.Key.ID)
				must(err)
				if job.Status == media.StatusCompleted || job.Status == media.StatusFailed {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("extension did not reach terminal state")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if job.Status != media.StatusCompleted || job.Provider != string(account.ProviderBuild) ||
				job.AccountID != buildCredential.ID || job.ModelRouteID != buildRoute.ID ||
				job.Operation != scenario.operation || job.ResultAssetID == "" {
				t.Fatalf("unexpected extension result: status=%s provider=%s account=%d route=%d asset=%q error=%s",
					job.Status, job.Provider, job.AccountID, job.ModelRouteID, job.ResultAssetID, job.ErrorMessage)
			}
			response, err = http.Get(api.URL + "/v1/media/videos/" + job.ResultAssetID)
			must(err)
			content, err := io.ReadAll(response.Body)
			must(err)
			response.Body.Close()
			if response.StatusCode != 200 || !bytes.Equal(content, mp4) {
				t.Fatalf("anonymous output download status=%d bytes=%d", response.StatusCode, len(content))
			}
			if creates.Load() != 1 || polls.Load() != 1 || otherCalls.Load() != 0 {
				t.Fatalf("creates=%d polls=%d unexpected=%d", creates.Load(), polls.Load(), otherCalls.Load())
			}
		})
	}
}
