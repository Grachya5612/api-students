package service

import (
	"strings"

	"api-students/app/model"
)

// ApplyStudentPatch menyalin field yang dikirim ke data student yang sudah ada.
// OwnerID sengaja tidak berasal dari request sehingga ownership tidak dapat
// diubah melalui PATCH.
// Validasi bentuk sudah selesai dikerjakan tag sebelum fungsi ini dipanggil,
// sehingga di sini tugasnya tinggal satu: menggabungkan.
func ApplyStudentPatch(
	current model.Student,
	req model.StudentPatchRequest,
) model.Student {
	if req.NIM != nil {
		current.NIM = strings.TrimSpace(*req.NIM)
	}

	if req.Name != nil {
		current.Name = strings.TrimSpace(*req.Name)
	}

	if req.Grade != nil {
		current.Grade = *req.Grade
	}

	if req.IsActive != nil {
		current.IsActive = *req.IsActive
	}

	return current
}

// IsEmptyStudentPatch menandai PATCH yang tidak mengubah apa pun.
// Aturan ini tidak dapat ditulis sebagai tag: tag memeriksa satu field
// pada satu waktu, sedangkan aturan ini berbicara tentang HUBUNGAN antar
// field — setidaknya satu di antara mereka harus ada.
func IsEmptyStudentPatch(req model.StudentPatchRequest) bool {
	return req.NIM == nil &&
		req.Name == nil &&
		req.Grade == nil &&
		req.IsActive == nil
}