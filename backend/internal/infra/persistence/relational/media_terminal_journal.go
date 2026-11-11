package relational

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/chenyme/grok2api/backend/internal/domain/media"
	"github.com/chenyme/grok2api/backend/internal/repository"
)

// ConfigureTerminalJournal 将终态收据放到媒体持久卷，重启后仍可恢复。
func (r *MediaJobRepository) ConfigureTerminalJournal(mediaPath string) error {
	dir := filepath.Join(mediaPath, ".video-terminals")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建视频终态收据目录: %w", err)
	}
	r.terminalDir = dir
	return nil
}

func terminalReceiptName(value media.Job) string {
	sum := sha256.Sum256([]byte(value.ID + "\x00" + value.ClaimToken))
	return hex.EncodeToString(sum[:]) + ".json"
}

// writeTerminalMediaJob 先同步原子收据，再执行带认领令牌校验的数据库写入。
func (r *MediaJobRepository) writeTerminalMediaJob(ctx context.Context, value media.Job) error {
	r.terminalMu.Lock()
	defer r.terminalMu.Unlock()
	value.InputJSON = ""
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(r.terminalDir, ".pending-")
	if err != nil {
		return err
	}
	temp := file.Name()
	defer os.Remove(temp)
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	path := filepath.Join(r.terminalDir, terminalReceiptName(value))
	if err := os.Rename(temp, path); err != nil {
		return err
	}
	dir, err := os.Open(r.terminalDir)
	if err != nil {
		return err
	}
	syncErr = dir.Sync()
	closeErr = dir.Close()
	if err := errors.Join(syncErr, closeErr); err != nil {
		return err
	}
	return r.applyTerminalReceipt(ctx, path, value)
}

// applyTerminalReceipt 不覆盖已有终态或其他执行者的认领，处理提交成功但应答丢失。
func (r *MediaJobRepository) applyTerminalReceipt(ctx context.Context, path string, value media.Job) error {
	var stored mediaJobModel
	err := r.db.db.WithContext(ctx).Select("id", "claim_token", "status").
		Where("id = ?", value.ID).First(&stored).Error
	if err != nil && !errors.Is(mapError(err), repository.ErrNotFound) {
		return err
	}
	stale := err != nil || stored.ClaimToken != value.ClaimToken
	terminal := stored.Status == string(media.StatusCompleted) || stored.Status == string(media.StatusFailed)
	if !stale && !terminal {
		if err := r.updateMediaJob(ctx, value); err != nil {
			return err
		}
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Warn("video_terminal_receipt_remove_failed", "job_id", value.ID, "error", err)
	}
	if stale {
		return repository.ErrNotFound
	}
	return nil
}

// flushTerminalMediaJobs 优先恢复终态；数据库仍不可用时停止认领，防止重新提交上游。
func (r *MediaJobRepository) flushTerminalMediaJobs(ctx context.Context) error {
	if r.terminalDir == "" {
		return nil
	}
	r.terminalMu.Lock()
	defer r.terminalMu.Unlock()
	entries, err := os.ReadDir(r.terminalDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(r.terminalDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var value media.Job
		if err := json.Unmarshal(data, &value); err != nil {
			return fmt.Errorf("读取视频终态收据 %s: %w", entry.Name(), err)
		}
		if value.ID == "" || entry.Name() != terminalReceiptName(value) ||
			(value.Status != media.StatusCompleted && value.Status != media.StatusFailed) {
			return fmt.Errorf("视频终态收据无效: %s", entry.Name())
		}
		if err := r.applyTerminalReceipt(ctx, path, value); err != nil && !errors.Is(err, repository.ErrNotFound) {
			return err
		}
		slog.Info("video_terminal_receipt_recovered", "job_id", value.ID)
	}
	return nil
}
