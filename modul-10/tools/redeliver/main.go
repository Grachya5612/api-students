package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/hibiken/asynq"

	"api-students/app/model"
	"api-students/config"
	"api-students/database"
)

// Alat uji Modul 11 (C.5 dan D.2): menyalin task dari satu event outbox ke Redis.
// Tiap salinan TIDAK diberi TaskID, supaya Redis menerima semua salinan.
// Untuk event course.quota_changed, salinan dibuat per penerima (fan-out),
// sama seperti yang dilakukan relay.
func main() {
	outboxID := flag.Int64("outbox", 0, "id event di tabel outbox")
	copies := flag.Int("copies", 1, "jumlah salinan task per penerima")
	flag.Parse()

	if *outboxID == 0 {
		log.Fatal("flag -outbox wajib diisi")
	}

	config.LoadEnv()
	ctx := context.Background()

	pool, err := database.NewPool(ctx)
	if err != nil {
		log.Fatalf("koneksi database: %v", err)
	}
	defer pool.Close()

	var eventType string
	var payload []byte
	err = pool.QueryRow(ctx,
		"SELECT event_type, payload FROM outbox WHERE id = $1", *outboxID,
	).Scan(&eventType, &payload)
	if err != nil {
		log.Fatalf("membaca outbox %d: %v", *outboxID, err)
	}

	// Daftar penerima: satu per peserta untuk event kuota, satu (0) untuk enrollment.
	var uids []int
	if eventType == "course.quota_changed" {
		var ev model.QuotaChangedEvent
		if err := json.Unmarshal(payload, &ev); err != nil {
			log.Fatalf("payload quota: %v", err)
		}
		rows, err := pool.Query(ctx,
			"SELECT user_id FROM enrollments WHERE course_id = $1", ev.CourseID)
		if err != nil {
			log.Fatalf("membaca peserta: %v", err)
		}
		for rows.Next() {
			var id int
			if err := rows.Scan(&id); err != nil {
				log.Fatalf("membaca peserta: %v", err)
			}
			uids = append(uids, id)
		}
		rows.Close()
	} else {
		uids = []int{0}
	}

	client := asynq.NewClient(config.RedisOpt())
	defer client.Close()

	total := 0
	for _, uid := range uids {
		job := model.NotificationJob{
			OutboxID:    *outboxID,
			EventType:   eventType,
			Payload:     json.RawMessage(payload),
			RecipientID: uid,
		}
		body, err := json.Marshal(job)
		if err != nil {
			log.Fatalf("marshal job: %v", err)
		}
		for i := 0; i < *copies; i++ {
			task := asynq.NewTask(model.TaskSendNotification, body,
				asynq.Queue(model.QueueNotifications),
				asynq.MaxRetry(5),
				asynq.Timeout(30*time.Second),
			)
			if _, err := client.Enqueue(task); err != nil {
				log.Fatalf("enqueue: %v", err)
			}
			total++
		}
	}

	fmt.Printf("%d salinan task untuk outbox %d masuk ke Redis\n", total, *outboxID)
}