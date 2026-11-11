//go:build integration

package web

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
	infraegress "github.com/chenyme/grok2api/backend/internal/infra/egress"
	"github.com/chenyme/grok2api/backend/internal/infra/persistence/relational"
	"github.com/chenyme/grok2api/backend/internal/infra/provider"
	"github.com/chenyme/grok2api/backend/internal/infra/security"
)

// TestIntegrationUnsignedWebMedia 通过真实数据库、出口管理器和 HTTP 核对媒体协议及拒绝边界。
func TestIntegrationUnsignedWebMedia(t *testing.T) {
	ctx := context.Background()
	db, err := relational.OpenSQLite(ctx, filepath.Join(t.TempDir(), "media.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.InitializeSchema(ctx); err != nil {
		t.Fatal(err)
	}
	cipher, err := security.NewCipher(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatal(err)
	}
	token, err := cipher.Encrypt("integration-only")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	creates, quotas := 0, 0
	rejection := http.StatusTooManyRequests
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("x-statsig-id") != "" {
			t.Error("media request carried obsolete signature")
		}
		switch r.URL.Path {
		case "/rest/media/imagine/quota_info":
			quotas++
			_, _ = io.WriteString(w, `{"image":null,"imagePro":{"available":false,"remainingQueries":0,"windowSizeSeconds":86400},"imageEdit":null,"video":null,"video720p":{"available":false,"remainingQueries":0,"windowSizeSeconds":86400}}`)
		case "/rest/app-chat/conversations/new":
			creates++
			w.WriteHeader(rejection)
			if rejection == http.StatusForbidden {
				_, _ = io.WriteString(w, `{"error":{"code":7,"message":"This page is out of date"}}`)
			} else {
				_, _ = io.WriteString(w, `{"error":{"code":8,"message":"Too many requests","details":[]}}`)
			}
		default:
			t.Errorf("unexpected signing or metadata request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	manager := infraegress.NewManager(relational.NewEgressRepository(db), cipher)
	adapter := NewAdapter(Config{
		BaseURL: server.URL, StatsigMode: "url", StatsigSignerURL: server.URL + "/obsolete-signer",
		VideoTimeoutSeconds: 5, QuotaTimeoutSeconds: 5,
	}, manager, cipher, nil, nil)
	credential := account.Credential{
		ID: 1, Provider: account.ProviderWeb, AuthType: account.AuthTypeSSO,
		WebTier: account.WebTierBasic, EncryptedAccessToken: token,
	}
	snapshot, err := adapter.SyncQuotaGroup(ctx, credential, account.QuotaGroupWebImagine)
	if err != nil {
		t.Fatal(err)
	}
	for _, window := range snapshot.Windows {
		if window.Remaining != 0 {
			t.Fatalf("unavailable product gained quota: %+v", window)
		}
	}
	for _, status := range []int{http.StatusTooManyRequests, http.StatusForbidden} {
		mu.Lock()
		rejection = status
		mu.Unlock()
		result, err := adapter.GenerateVideo(ctx, provider.VideoRequest{
			Credential: credential, Model: "grok-imagine-video-1.5",
			Prompt: "A red ceramic teapot.", Duration: 6, Resolution: "720p",
		})
		if err == nil || provider.ErrorHTTPStatusOrZero(err) != status {
			t.Fatalf("status=%d result=%+v error=%v", status, result, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if quotas != 1 || creates != 2 {
		t.Fatalf("quota calls=%d generation calls=%d; rejected POST must not replay", quotas, creates)
	}
}
