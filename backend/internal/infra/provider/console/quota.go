package console

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/chenyme/grok2api/backend/internal/domain/account"
	egressdomain "github.com/chenyme/grok2api/backend/internal/domain/egress"
	"github.com/chenyme/grok2api/backend/internal/infra/provider"
)

const (
	consoleQuotaTimeout = 45 * time.Second
	// consoleWeeklyWindow is INCLUDED_USAGE_PERIOD_WEEKLY; the exact reset
	// instant comes from resetsAt and is aligned to account creation.
	consoleWeeklyWindow = 7 * 24 * time.Hour
)

func (a *Adapter) SyncQuota(ctx context.Context, credential account.Credential) (provider.QuotaSnapshot, error) {
	windows, syncedAt, err := a.syncConsoleQuotas(ctx, credential)
	if err != nil {
		return provider.QuotaSnapshot{}, err
	}
	return provider.QuotaSnapshot{Windows: windows, SyncedAt: syncedAt}, nil
}

func (a *Adapter) SyncQuotaMode(ctx context.Context, credential account.Credential, mode string) (account.QuotaWindow, error) {
	if !isConsoleQuotaMode(mode) {
		return account.QuotaWindow{}, fmt.Errorf("不支持的 Console 额度模式 %q", mode)
	}
	windows, _, err := a.syncConsoleQuotas(ctx, credential)
	if err != nil {
		return account.QuotaWindow{}, err
	}
	for _, window := range windows {
		if window.Mode == mode {
			return window, nil
		}
	}
	return account.QuotaWindow{}, fmt.Errorf("Console usage 响应缺少 %s 额度", consoleQuotaKind(mode))
}

// syncConsoleQuotas 经账号所属 Console 出口读取 grok.com 周额度，三个媒体模式共享预算。
func (a *Adapter) syncConsoleQuotas(ctx context.Context, credential account.Credential) ([]account.QuotaWindow, time.Time, error) {
	ssoToken, err := a.cipher.Decrypt(credential.EncryptedAccessToken)
	if err != nil {
		return nil, time.Time{}, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, consoleQuotaTimeout)
	defer cancel()
	// 查询域名不改变账号的出口授权，绑定 Console 节点不能被当作 Web 节点使用。
	lease, err := a.egress.AcquireCredential(requestCtx, egressdomain.ScopeConsole, credential)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer lease.Release()
	if remaining := a.breaker.blocked(lease.NodeID); remaining > 0 {
		return nil, time.Time{}, &egressBlockedError{retryAfter: remaining}
	}
	usage, err := a.fetchConsoleUsage(requestCtx, credential, ssoToken, lease)
	if err != nil {
		if errors.Is(err, errCloudflareRejected) {
			delay, opened := a.breaker.trip(lease.NodeID)
			if opened {
				a.logger().Warn("console_quota_egress_blocked", "egress_node_id", lease.NodeID, "account_id", credential.ID, "pause", delay, "error", err)
			}
			return nil, time.Time{}, &egressBlockedError{retryAfter: delay}
		}
		if !errors.Is(err, provider.ErrUnauthorized) {
			a.egress.FeedbackForScope(context.WithoutCancel(ctx), egressdomain.ScopeConsole, lease.NodeID, 0, err)
		}
		return nil, time.Time{}, err
	}
	a.breaker.reset(lease.NodeID)
	a.egress.FeedbackForScope(context.WithoutCancel(ctx), egressdomain.ScopeConsole, lease.NodeID, http.StatusOK, nil)
	now := time.Now().UTC()
	return consoleQuotaWindows(credential.ID, usage, now), now, nil
}

func consoleQuotaWindows(accountID uint64, usage consoleUsageStatus, now time.Time) []account.QuotaWindow {
	remaining := usage.BudgetCents - usage.UsedCents
	if remaining < 0 {
		// The upstream records cost after execution, so one large call can
		// overshoot the weekly budget several times.
		remaining = 0
	}
	usagePercent := 100.0
	if usage.BudgetCents > 0 {
		usagePercent = math.Min(100, float64(usage.UsedCents)/float64(usage.BudgetCents)*100)
	}
	var resetAt *time.Time
	if usage.ResetsAt != nil {
		value := usage.ResetsAt.UTC()
		resetAt = &value
	}
	windows := make([]account.QuotaWindow, 0, 3)
	for _, mode := range []string{QuotaMode, QuotaModeImage, QuotaModeVideo} {
		var modeResetAt *time.Time
		if resetAt != nil {
			value := *resetAt
			modeResetAt = &value
		}
		syncedAt := now
		windows = append(windows, account.QuotaWindow{
			AccountID: accountID, Mode: mode, Remaining: remaining, Total: usage.BudgetCents,
			UsagePercent: usagePercent, WindowSeconds: int(consoleWeeklyWindow / time.Second), ResetAt: modeResetAt,
			SyncedAt: &syncedAt, Source: account.QuotaSourceUpstream, UpdatedAt: now,
		})
	}
	return windows
}

func isConsoleQuotaMode(mode string) bool {
	return mode == QuotaMode || mode == QuotaModeImage || mode == QuotaModeVideo
}

func consoleQuotaMode(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "chat":
		return QuotaMode
	case "image":
		return QuotaModeImage
	case "video":
		return QuotaModeVideo
	default:
		return ""
	}
}

func consoleQuotaKind(mode string) string {
	switch mode {
	case QuotaMode:
		return "chat"
	case QuotaModeImage:
		return "image"
	case QuotaModeVideo:
		return "video"
	default:
		return mode
	}
}
