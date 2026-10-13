package handlers

import (
	"context"
	"strings"
	"time"

	"github.com/adolgina/fitness-helper/internal/chat"
	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/keyboards"
	"github.com/adolgina/fitness-helper/internal/view"
)

// weeklyBackupCheckInterval is how often due backups are looked for;
// who is due is decided by the use case.
const weeklyBackupCheckInterval = time.Hour

func (b *Bot) showBackup(ctx context.Context, t target, userID int64) {
	settings, err := b.svc.BackupSettings(ctx, userID)
	if err != nil {
		b.fail(ctx, t, err)
		return
	}
	b.show(ctx, t, view.Backup(settings), keyboards.Backup(settings))
}

func (b *Bot) onWeeklyBackup(ctx context.Context, t target, userID int64, enabled bool) {
	if err := b.svc.SetWeeklyBackup(ctx, userID, enabled); err != nil {
		b.fail(ctx, t, err)
		return
	}
	b.showBackup(ctx, t, userID)
}

func (b *Bot) onExport(ctx context.Context, t target, userID int64) {
	data, name, err := b.svc.ExportUserData(ctx, userID)
	if err != nil {
		b.fail(ctx, t, err)
		return
	}
	if err := b.tg.SendDocument(ctx, t.chatID, name, data, view.ExportCaption); err != nil {
		b.fail(ctx, t, err)
	}
}

func (b *Bot) onDocument(ctx context.Context, m *chat.Message) {
	t := newMessage(m.ChatID)
	if !strings.HasSuffix(strings.ToLower(m.Document.FileName), ".zip") {
		b.say(ctx, t.chatID, view.ImportHint)
		return
	}
	data, err := b.tg.Download(ctx, *m.Document)
	if err != nil {
		b.fail(ctx, t, err)
		return
	}
	b.setPendingImport(m.UserID, data)
	b.show(ctx, t, view.ImportAsk, keyboards.ImportConfirm())
}

func (b *Bot) onImport(ctx context.Context, t target, userID int64, mode domain.ImportMode) {
	data, ok := b.takePendingImport(userID)
	if !ok {
		b.show(ctx, t, view.ImportNoFile, keyboards.Menu())
		return
	}
	res, err := b.svc.ImportUserData(ctx, userID, data, mode)
	if err != nil {
		b.log.Error("import", "err", err)
		b.show(ctx, t, view.ImportFailed, keyboards.Menu())
		return
	}
	b.show(ctx, t, view.ImportResult(res), keyboards.Menu())
}

func (b *Bot) setPendingImport(userID int64, data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pendingImport[userID] = data
}

func (b *Bot) takePendingImport(userID int64) ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	data, ok := b.pendingImport[userID]
	delete(b.pendingImport, userID)
	return data, ok
}

// RunWeeklyBackup sends due weekly backups until ctx is cancelled.
func (b *Bot) RunWeeklyBackup(ctx context.Context) {
	ticker := time.NewTicker(weeklyBackupCheckInterval)
	defer ticker.Stop()
	for {
		b.runWeeklyBackupOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (b *Bot) runWeeklyBackupOnce(ctx context.Context) {
	users, err := b.svc.ListUsersDueWeeklyBackup(ctx)
	if err != nil {
		b.log.Error("weekly backup list", "err", err)
		return
	}
	for _, uid := range users {
		data, name, err := b.svc.ExportUserData(ctx, uid)
		if err != nil {
			b.log.Error("weekly backup export", "user", uid, "err", err)
			continue
		}
		if err := b.tg.SendDocument(ctx, uid, name, data, view.WeeklyCaption); err != nil {
			b.log.Error("weekly backup send", "user", uid, "err", err)
			continue
		}
		if err := b.svc.MarkBackupSent(ctx, uid); err != nil {
			b.log.Error("weekly backup mark", "user", uid, "err", err)
		}
	}
}
