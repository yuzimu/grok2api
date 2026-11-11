//go:build integration

package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	accountapp "github.com/chenyme/grok2api/backend/internal/application/account"
	clientkeyapp "github.com/chenyme/grok2api/backend/internal/application/clientkey"
	"github.com/chenyme/grok2api/backend/internal/domain/account"
	"github.com/chenyme/grok2api/backend/internal/domain/clientkey"
	"github.com/chenyme/grok2api/backend/internal/domain/media"
	infraegress "github.com/chenyme/grok2api/backend/internal/infra/egress"
	"github.com/chenyme/grok2api/backend/internal/infra/persistence/relational"
	"github.com/chenyme/grok2api/backend/internal/infra/provider"
	webprovider "github.com/chenyme/grok2api/backend/internal/infra/provider/web"
	"github.com/chenyme/grok2api/backend/internal/infra/runtime/memory"
)

// TestIntegrationWebUploadHTTPRecovery 通过真实 SQLite、选号器、Web 适配器和 HTTP 上游验证重试边界。
func TestIntegrationWebUploadHTTPRecovery(t *testing.T) {
	for _, scenario := range []struct {
		name         string
		uploadStatus int
		createStatus int
		attempts     int
		pinned       bool
		wantUploads  int
		wantCreates  int
		wantSecond   bool
	}{
		{"upload_500_recovers", 500, 400, 2, false, 2, 1, true},
		{"global_limit_one", 500, 400, 1, false, 1, 0, false},
		{"pinned_account", 500, 400, 2, true, 1, 0, false},
		{"upload_400_not_retried", 400, 400, 2, false, 1, 0, false},
		{"submitted_500_not_replayed", 200, 500, 2, false, 1, 1, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "integration.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := db.InitializeSchema(ctx); err != nil {
				t.Fatal(err)
			}
			var mu sync.Mutex
			var uploads []string
			creates := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/http/upload-file-v2/direct":
					cookie, err := r.Cookie("sso")
					if err != nil {
						t.Error(err)
						w.WriteHeader(401)
						return
					}
					if err := r.ParseMultipartForm(1 << 20); err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					defer r.MultipartForm.RemoveAll()
					if r.FormValue("file_source") != "IMAGINE_SELF_UPLOAD_FILE_SOURCE" ||
						len(r.MultipartForm.File["file"]) != 1 {
						t.Error("reference upload contract changed")
					}
					uploads = append(uploads, cookie.Value)
					if len(uploads) == 1 && scenario.uploadStatus != 200 {
						w.WriteHeader(scenario.uploadStatus)
						_, _ = io.WriteString(w, `{"error":{"code":13,"message":"Internal: Failed to store uploaded file"}}`)
						return
					}
					_, _ = io.WriteString(w, `{"uploadId":"upload","fileMetadata":{"fileMetadataId":"reference","fileUri":"https://assets.grok.com/reference.jpg"}}`)
				case "/rest/media/post/create":
					_, _ = io.WriteString(w, `{"post":{"id":"reference-post"}}`)
				case "/rest/app-chat/conversations/new":
					creates++
					// 固定拒绝生成请求，只验证上传恢复，不创建真实视频。
					w.WriteHeader(scenario.createStatus)
					_, _ = io.WriteString(w, `{"error":{"message":"controlled generation rejection"}}`)
				default:
					t.Errorf("unexpected upstream request: %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer upstream.Close()

			cipher := testCipher(t)
			accounts := relational.NewAccountRepository(db)
			models := relational.NewModelRepository(db)
			audits := relational.NewAuditRepository(db)
			jobs := relational.NewMediaJobRepository(db)
			keys := relational.NewClientKeyRepository(db)
			key, err := keys.Create(ctx, clientkey.Key{
				Name: "integration", Prefix: "integration", SecretHash: strings.Repeat("a", 64),
				EncryptedSecret: "unused", Enabled: true, RPMLimit: 60, MaxConcurrent: 4,
			})
			if err != nil {
				t.Fatal(err)
			}
			var credentials []account.Credential
			for i, name := range []string{"first", "second"} {
				token, err := cipher.Encrypt(name)
				if err != nil {
					t.Fatal(err)
				}
				credential, _, err := accounts.UpsertByIdentity(ctx, account.Credential{
					Provider: account.ProviderWeb, AuthType: account.AuthTypeSSO, WebTier: account.WebTierSuper,
					Name: name, SourceKey: name, EncryptedAccessToken: token, ExpiresAt: time.Now().Add(time.Hour),
					Enabled: true, AuthStatus: account.AuthStatusActive, Priority: 200 - i*100, MaxConcurrent: 1,
				})
				if err != nil {
					t.Fatal(err)
				}
				credentials = append(credentials, credential)
			}
			const model = "grok-imagine-video-1.5"
			if err := models.UpsertDiscovered(ctx, account.ProviderWeb, []string{model}); err != nil {
				t.Fatal(err)
			}
			for _, credential := range credentials {
				if err := models.ReplaceAccountCapabilities(ctx, credential.ID, []string{model}, time.Now()); err != nil {
					t.Fatal(err)
				}
			}
			route, err := models.GetByProviderUpstream(ctx, account.ProviderWeb, model)
			if err != nil {
				t.Fatal(err)
			}
			egress := infraegress.NewManager(relational.NewEgressRepository(db), cipher)
			adapter := webprovider.NewAdapter(webprovider.Config{
				BaseURL: upstream.URL, StatsigMode: "manual", StatsigManualValue: "integration",
				VideoTimeoutSeconds: 10,
			}, egress, cipher, nil, nil)
			registry := provider.NewRegistry(adapter)
			sticky := memory.NewStickyStore()
			accountService := accountapp.NewService(accounts, audits, memory.NewDeviceSessionStore(), sticky, registry, cipher, nil)
			selector := NewSelector(accounts, memory.NewConcurrencyLimiter(), sticky, registry, time.Hour, time.Second, time.Minute)
			service := NewService(models, audits, accountService, clientkeyapp.NewService(keys, nil, nil, 60, 4, nil), registry, selector, nil, 3)
			service.ConfigureMedia(jobs, 1)
			service.UpdateVideoMaxAttempts(scenario.attempts)
			input, err := json.Marshal(map[string]any{
				"reference_urls": []string{"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aXioAAAAASUVORK5CYII="},
				"pinned_only":    scenario.pinned,
			})
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			job := media.Job{
				ID: "video_integration", RequestID: "upload-integration", ClientKeyID: key.ID, ClientKeyName: key.Name,
				AccountID: credentials[0].ID, AccountName: credentials[0].Name, Provider: string(account.ProviderWeb),
				Model: route.PublicID, ModelRouteID: route.ID, UpstreamModel: model,
				Operation: provider.VideoOperationGenerate, Prompt: "integration", Seconds: 6, Quality: "720p",
				Status: media.StatusInProgress, InputJSON: string(input), CreatedAt: now, UpdatedAt: now,
			}
			if err := jobs.CreateMediaJob(ctx, job); err != nil {
				t.Fatal(err)
			}
			service.runVideoJob(ctx, job, route)
			mu.Lock()
			defer mu.Unlock()
			if len(uploads) != scenario.wantUploads || creates != scenario.wantCreates {
				t.Fatalf("uploads=%v creates=%d, want uploads=%d creates=%d", uploads, creates, scenario.wantUploads, scenario.wantCreates)
			}
			if scenario.wantSecond && (uploads[0] != "first" || uploads[1] != "second") {
				t.Fatalf("upload recovery did not switch accounts: %v", uploads)
			}
			stored, err := jobs.GetMediaJob(ctx, job.ID, key.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Status != media.StatusFailed {
				t.Fatalf("controlled upstream rejection did not finish the job: %s", stored.Status)
			}
		})
	}
}
