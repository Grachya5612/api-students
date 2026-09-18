package service

import (
	"strings"

	"api-students/app/model"
)

// ValidateStudentCreate memeriksa isi permintaan POST /students.
func ValidateStudentCreate(req model.StudentCreateRequest) map[string]string {
	errs := map[string]string{}

	if strings.TrimSpace(req.NIM) == "" {
		errs["nim"] = "wajib diisi"
	}

	if strings.TrimSpace(req.Name) == "" {
		errs["name"] = "wajib diisi"
	}

	if req.Grade < 0 || req.Grade > 100 {
		errs["grade"] = "harus berada di antara 0 dan 100"
	}

	return errs
}

// ValidateStudentReplace memeriksa isi permintaan PUT /students/:id.
func ValidateStudentReplace(req model.StudentUpdateRequest) map[string]string {
	errs := map[string]string{}

	if strings.TrimSpace(req.NIM) == "" {
		errs["nim"] = "wajib diisi pada PUT"
	}

	if strings.TrimSpace(req.Name) == "" {
		errs["name"] = "wajib diisi pada PUT"
	}

	if req.Grade < 0 || req.Grade > 100 {
		errs["grade"] = "harus berada di antara 0 dan 100"
	}

	return errs
}

// ApplyStudentPatch menyalin field yang dikirim ke data student yang sudah ada.
// OwnerID sengaja tidak berasal dari request sehingga ownership tidak dapat
// diubah melalui PATCH.
func ApplyStudentPatch(
	current model.Student,
	req model.StudentPatchRequest,
) (model.Student, map[string]string) {
	errs := map[string]string{}

	if req.NIM != nil {
		if strings.TrimSpace(*req.NIM) == "" {
			errs["nim"] = "tidak boleh kosong"
		} else {
			current.NIM = strings.TrimSpace(*req.NIM)
		}
	}

	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			errs["name"] = "tidak boleh kosong"
		} else {
			current.Name = strings.TrimSpace(*req.Name)
		}
	}

	if req.Grade != nil {
		if *req.Grade < 0 || *req.Grade > 100 {
			errs["grade"] = "harus berada di antara 0 dan 100"
		} else {
			current.Grade = *req.Grade
		}
	}

	if req.IsActive != nil {
		current.IsActive = *req.IsActive
	}

	return current, errs
}

// IsEmptyStudentPatch menandai PATCH yang tidak mengubah apa pun.
func IsEmptyStudentPatch(req model.StudentPatchRequest) bool {
	return req.NIM == nil &&
		req.Name == nil &&
		req.Grade == nil &&
		req.IsActive == nil
}