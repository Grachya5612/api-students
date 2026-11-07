package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"api-students/app/model"
	"api-students/app/repository"

	"github.com/hibiken/asynq"
)

// TaskEnqueuer adalah kemampuan melakukan enqueue task.
// *asynq.Client memenuhi interface ini.
type TaskEnqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

type CourseEnrollmentReader interface {
	EnrolledUserIDs(ctx context.Context, courseID int) ([]int, error)
}

// OutboxRelay memindahkan event dari tabel outbox ke queue Redis.
//
// Relay adalah satu-satunya komponen yang membaca outbox. API tidak pernah
// berbicara dengan Redis; ia cukup menulis ke PostgreSQL. Karena itu
// mahasiswa tetap dapat mengisi KRS walaupun Redis sedang mati — notifikasinya
// hanya tertunda, tidak hilang.
type OutboxRelay struct {
	outbox   repository.OutboxRepository
	tx       *repository.TxManager
	queue    TaskEnqueuer
	courses  CourseEnrollmentReader
	logger   *slog.Logger
	interval time.Duration
	batch    int
}

func NewOutboxRelay(
	outbox repository.OutboxRepository,
	tx *repository.TxManager,
	queue TaskEnqueuer,
	logger *slog.Logger,
	interval time.Duration,
	batch int,
	courses ...CourseEnrollmentReader,
) *OutboxRelay {
	relay := &OutboxRelay{
		outbox:   outbox,
		tx:       tx,
		queue:    queue,
		logger:   logger,
		interval: interval,
		batch:    batch,
	}
	if len(courses) > 0 {
		relay.courses = courses[0]
	}
	return relay
}

// Run berjalan terus sampai ctx dibatalkan.
func (r *OutboxRelay) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		n, err := r.RelayOnce(ctx)
		if err != nil {
			r.logger.Warn("relay_delayed",
				slog.Int("published", n),
				slog.String("error", err.Error()))
		} else if n > 0 {
			r.logger.Info("relay_done", slog.Int("published", n))
		}
		// Satu batch penuh berarti kemungkinan besar masih ada sisa:
		// langsung lanjutkan tanpa menunggu. Selain itu, tunggu giliran
		// berikutnya.
		if err == nil && n == r.batch && ctx.Err() == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RelayOnce mem-publish paling banyak satu batch. Mengembalikan jumlah
// event yang berhasil di-publish.
func (r *OutboxRelay) RelayOnce(ctx context.Context) (int, error) {
	var published int
	var enqueueErr error
	err := r.tx.WithTxOnce(ctx, func(tx repository.DBTX) error {
		repo := r.outbox.WithTx(tx)
		events, err := repo.FetchUnpublished(ctx, r.batch)
		if err != nil {
			return err
		}
		ids := make([]int64, 0, len(events))
		for _, e := range events {
			if err := r.publish(ctx, e); err != nil {
				enqueueErr = err
				break
			}
			ids = append(ids, e.ID)
		}
		// Hanya event yang SUDAH berhasil di-enqueue yang ditandai.
		if err := repo.MarkPublished(ctx, ids); err != nil {
			return err
		}
		published = len(ids)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return published, enqueueErr
}

func (r *OutboxRelay) publish(ctx context.Context, e model.OutboxEvent) error {
	if e.EventType == "course.quota_changed" {
		return r.publishQuotaChanged(ctx, e)
	}

	job := model.NotificationJob{
		OutboxID:  e.ID,
		EventType: e.EventType,
		Payload:   e.Payload,
	}

	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal notification job %d: %w", e.ID, err)
	}

	task := asynq.NewTask(model.TaskSendNotification, payload,
		// TaskID yang diturunkan dari id outbox membuat enqueue ulang
		// event yang sama ditolak Redis. Ini menutup celah ketika relay
		// mati SETELAH melakukan enqueue tetapi SEBELUM sempat commit.
		asynq.TaskID(fmt.Sprintf("outbox-%d", e.ID)),
		asynq.Queue(model.QueueNotifications),
		asynq.MaxRetry(5),
		asynq.Timeout(30*time.Second),
		// Task yang selesai disimpan 24 jam. Selama itu, TaskID-nya
		// tetap tercatat sehingga perlindungan di atas tetap berlaku.
		asynq.Retention(24*time.Hour),
	)
	_, err = r.queue.EnqueueContext(ctx, task)
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		// Sudah pernah di-enqueue pada putaran sebelumnya. Bagi relay,
		// itu sama dengan berhasil.
		return nil
	}
	if err != nil {
		return fmt.Errorf("enqueue outbox %d: %w", e.ID, err)
	}
	return nil
}

func (r *OutboxRelay) publishQuotaChanged(ctx context.Context, e model.OutboxEvent) error {
	if r.courses == nil {
		return errors.New("course enrollment reader is not configured")
	}
	var ev model.QuotaChangedEvent
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		return fmt.Errorf("payload quota %d: %w", e.ID, err)
	}
	uids, err := r.courses.EnrolledUserIDs(ctx, int(ev.CourseID))
	if err != nil {
		return err
	}
	for _, uid := range uids {
		job := model.NotificationJob{
			OutboxID:    e.ID,
			EventType:   e.EventType,
			Payload:     e.Payload,
			RecipientID: int(uid),
		}
		body, err := json.Marshal(job)
		if err != nil {
			return fmt.Errorf("marshal quota %d penerima %d: %w", e.ID, uid, err)
		}
		task := asynq.NewTask(model.TaskSendNotification, body,
			asynq.TaskID(fmt.Sprintf("outbox-%d-u%d", e.ID, uid)),
			asynq.Queue(model.QueueNotifications),
			asynq.MaxRetry(5),
			asynq.Timeout(30*time.Second),
			asynq.Retention(24*time.Hour),
		)
		if _, err := r.queue.EnqueueContext(ctx, task); err != nil &&
			!errors.Is(err, asynq.ErrTaskIDConflict) {
			return fmt.Errorf("enqueue quota %d penerima %d: %w", e.ID, uid, err)
		}
	}
	return nil
}
