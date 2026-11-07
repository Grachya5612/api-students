package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"api-students/app/model"
	"api-students/app/repository"
	"api-students/helper"

	"github.com/hibiken/asynq"
)

// NotificationWorker mengerjakan task "notification:send".
//
// Bagi worker, Asynq adalah framework-nya — sama seperti Fiber bagi API.
// Karena itu method di sini menerima *asynq.Task, seperti method service
// lain menerima *fiber.Ctx.
type NotificationWorker struct {
	users         repository.UserRepository
	notifications repository.NotificationRepository
	tx            *repository.TxManager
	mailer        helper.Mailer
	logger        *slog.Logger
}

func NewNotificationWorker(
	users repository.UserRepository,
	notifications repository.NotificationRepository,
	tx *repository.TxManager,
	mailer helper.Mailer,
	logger *slog.Logger,
) *NotificationWorker {
	return &NotificationWorker{
		users:         users,
		notifications: notifications,
		tx:            tx,
		mailer:        mailer,
		logger:        logger,
	}
}

// HandleSend mengembalikan error untuk meminta Asynq melakukan retry nanti,
// atau error yang membungkus asynq.SkipRetry untuk menyatakan "jangan di-retry,
// task ini tidak mungkin berhasil".
//
// Asynq menjamin setiap task dijalankan PALING SEDIKIT satu kali, bukan
// TEPAT satu kali. Task yang sama bisa datang dua kali, misalnya bila
// worker mati setelah mengirim email tetapi sebelum sempat melapor ke Redis.
// Karena itu handler ini harus idempoten: menjalankannya dua kali untuk
// task yang sama harus berakibat sama dengan menjalankannya sekali.
func (w *NotificationWorker) HandleSend(ctx context.Context, t asynq.Task) error {
	var job model.NotificationJob
	if err := json.Unmarshal(t.Payload(), &job); err != nil {
		return fmt.Errorf("%w: payload task rusak: %v", asynq.SkipRetry, err)
	}
		var recipientID int
	var event model.EnrollmentEvent
	if job.EventType == "course.quota_changed" {
		recipientID = job.RecipientID
	} else {
		if err := json.Unmarshal(job.Payload, &event); err != nil {
			return fmt.Errorf("%w: payload event %d rusak: %v", asynq.SkipRetry, job.OutboxID, err)
		}
		recipientID = event.UserID
	}

	user, err := w.users.FindByID(ctx, recipientID)
	if errors.Is(err, repository.ErrNotFound) {
		return fmt.Errorf("%w: user %d tidak ditemukan", asynq.SkipRetry, recipientID)
	}
	if err != nil {
		return err
	}

	var msg model.Message
	if job.EventType == "course.quota_changed" {
		var ev model.QuotaChangedEvent
		if err := json.Unmarshal(job.Payload, &ev); err != nil {
			return fmt.Errorf("%w: payload quota %d rusak: %v", asynq.SkipRetry, job.OutboxID, err)
		}
		msg = BuildQuotaMessage(ev, user)
	} else {
		msg, err = BuildMessage(job.EventType, event, user)
		if err != nil {
			return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
		}
	}
	
	alreadySent := false
	err = w.tx.WithTxOnce(ctx, func(tx repository.DBTX) error {
		// Claim dan pengiriman berada di dalam SATU transaction:
		// - pengiriman gagal -> claim ikut dibatalkan -> attempt
		// berikutnya masih dapat mengirim;
		// - pengiriman sukses -> claim di-commit -> attempt berikutnya
		// melihat claim ini dan tidak mengirim lagi.
		claimed, err := w.notifications.WithTx(tx).Claim(ctx, model.Notification{
			OutboxID: job.OutboxID,
			UserID:   user.ID,
			Channel:  "email",
			Subject:  msg.Subject,
			Body:     msg.Body,
		})
		if err != nil {
			return err
		}
		if !claimed {
			alreadySent = true
			return nil
		}
		return w.mailer.Send(ctx, msg)
	})

	if errors.Is(err, helper.ErrPermanentDelivery) {
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}
	if err != nil {
		return err
	}

	retry, _ := asynq.GetRetryCount(ctx)
	w.logger.Info("notification_processed",
		slog.Int64("outbox_id", job.OutboxID),
		slog.String("event", job.EventType),
		slog.Int("retry", retry),
		slog.Bool("already_sent", alreadySent))

	return nil
}
