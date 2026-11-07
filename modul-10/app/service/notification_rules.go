package service

import (
	"errors"
	"fmt"

	"api-students/app/model"
)

var errUnknownEvent = errors.New("jenis event tidak dikenal")

// BuildMessage menyusun pesan dari sebuah event. Fungsi murni: tidak
// menyentuh database, Redis, maupun email, sehingga mudah diuji.
func BuildMessage(eventType string, e model.EnrollmentEvent, user model.User) (model.Message, error) {
	course := e.CourseCode + " " + e.CourseName
	var subject, body string
	switch eventType {
	case model.EventEnrollmentCreated:
		subject = "KRS: berhasil terdaftar di " + course
		body = fmt.Sprintf("Halo %s, Anda tercatat sebagai peserta kelas %s.", user.Username, course)
	case model.EventEnrollmentDropped:
		subject = "KRS: keluar dari " + course
		body = fmt.Sprintf("Halo %s, Anda sudah tidak terdaftar di kelas %s.", user.Username, course)
	case model.EventEnrollmentSwapped:
		from := e.FromCourseCode + " " + e.FromCourseName
		subject = "KRS: pindah dari " + from + " ke " + course
		body = fmt.Sprintf("Halo %s, Anda telah pindah dari kelas %s ke kelas %s.", user.Username,
			from, course)
	default:
		return model.Message{}, fmt.Errorf("%w: %q", errUnknownEvent, eventType)
	}
	return model.Message{To: user.Email, Subject: subject, Body: body}, nil
}

// BuildQuotaMessage menyusun email perubahan kuota untuk satu penerima.
func BuildQuotaMessage(ev model.QuotaChangedEvent, user model.User) model.Message {
	return model.Message{
		To:      user.Email,
		Subject: fmt.Sprintf("KRS: kuota %s %s berubah menjadi %d", ev.Code, ev.Name, ev.Quota),
		Body:    fmt.Sprintf("Halo %s, kuota kelas %s %s kini %d.", user.Username, ev.Code, ev.Name, ev.Quota),
	}
}