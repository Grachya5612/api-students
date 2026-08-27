package main

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"api-students/app/model"
	"api-students/app/repository"
)

// StudentHandler tidak lagi memakai slice/variabel global.
// Kebutuhannya (repository) disuntikkan dari luar lewat NewStudentHandler.
type StudentHandler struct {
	repo repository.StudentRepository
}

// Perhatikan tipe parameternya: INTERFACE (repository.StudentRepository),
// bukan struct konkret. Handler tidak tahu dan tidak perlu tahu datanya
// disimpan di PostgreSQL, MongoDB, atau di mana pun.
func NewStudentHandler(repo repository.StudentRepository) *StudentHandler {
	return &StudentHandler{repo: repo}
}

// terjemahkanError memetakan error milik repository menjadi status HTTP.
// Satu tempat untuk seluruh handler, supaya pemetaannya tidak tercecer
// di banyak fungsi berbeda.
func terjemahkanError(c *fiber.Ctx, err error, pesanUmum string) error {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return fail(c, fiber.StatusNotFound, "mahasiswa dengan id tersebut tidak ditemukan")
	case errors.Is(err, repository.ErrDuplicate):
		return fail(c, fiber.StatusConflict, "NIM sudah terdaftar pada mahasiswa lain")
	default:
		return fail(c, fiber.StatusInternalServerError, pesanUmum)
	}
}

// validateStudentFields memvalidasi field WAJIB (dipakai POST & PUT).
func validateStudentFields(nim, name string, grade float64) map[string]string {
	errs := map[string]string{}
	if strings.TrimSpace(nim) == "" {
		errs["nim"] = "wajib diisi"
	}
	if strings.TrimSpace(name) == "" {
		errs["name"] = "wajib diisi"
	}
	if grade < 0 || grade > 4 {
		errs["grade"] = "harus di antara 0 dan 4"
	}
	return errs
}

// List menangani GET /api/v1/students
func (h *StudentHandler) List(c *fiber.Ctx) error {
	ctx, cancel := reqCtx(c)
	defer cancel()

	q := parseListQuery(c)

	students, total, err := h.repo.FindAll(ctx, q)
	if err != nil {
		return fail(c, fiber.StatusInternalServerError, "gagal mengambil data mahasiswa")
	}

	totalPages := 0
	if q.Limit > 0 {
		totalPages = (total + q.Limit - 1) / q.Limit
	}

	return okList(c, "daftar mahasiswa berhasil diambil", students, &model.Meta{
		Page: q.Page, Limit: q.Limit, Total: total, TotalPages: totalPages,
	})
}

// Get menangani GET /api/v1/students/:id
func (h *StudentHandler) Get(c *fiber.Ctx) error {
	ctx, cancel := reqCtx(c)
	defer cancel()

	id, valid := paramID(c)
	if !valid {
		return fail(c, fiber.StatusBadRequest, "id harus berupa angka positif")
	}

	student, err := h.repo.FindByID(ctx, id)
	if err != nil {
		return terjemahkanError(c, err, "gagal mengambil data mahasiswa")
	}
	return ok(c, "mahasiswa ditemukan", student)
}

// Create menangani POST /api/v1/students
func (h *StudentHandler) Create(c *fiber.Ctx) error {
	ctx, cancel := reqCtx(c)
	defer cancel()

	var req model.StudentCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return fail(c, fiber.StatusBadRequest, "body harus berupa JSON yang valid")
	}

	req.NIM = strings.TrimSpace(req.NIM)
	req.Name = strings.TrimSpace(req.Name)

	if errs := validateStudentFields(req.NIM, req.Name, req.Grade); len(errs) > 0 {
		return failValidation(c, errs)
	}

	// Keunikan NIM TIDAK diperiksa dengan SELECT lebih dulu di sini.
	// Basis data sudah menjaminnya lewat UNIQUE INDEX pada LOWER(nim),
	// dan pemeriksaan manual di Go justru menyisakan celah race condition
	// bila dua request datang nyaris bersamaan.
	baru, err := h.repo.Create(ctx, model.Student{
		NIM: req.NIM, Name: req.Name, Grade: req.Grade, IsActive: req.IsActive,
	})
	if err != nil {
		return terjemahkanError(c, err, "gagal menyimpan mahasiswa")
	}

	return created(c, "mahasiswa baru berhasil ditambahkan", baru,
		"/api/v1/students/"+strconv.Itoa(baru.ID))
}

// Replace menangani PUT /api/v1/students/:id (ganti SELURUH field)
func (h *StudentHandler) Replace(c *fiber.Ctx) error {
	ctx, cancel := reqCtx(c)
	defer cancel()

	id, valid := paramID(c)
	if !valid {
		return fail(c, fiber.StatusBadRequest, "id harus berupa angka positif")
	}

	var req model.StudentUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return fail(c, fiber.StatusBadRequest, "body harus berupa JSON yang valid")
	}

	req.NIM = strings.TrimSpace(req.NIM)
	req.Name = strings.TrimSpace(req.Name)

	if errs := validateStudentFields(req.NIM, req.Name, req.Grade); len(errs) > 0 {
		return failValidation(c, errs)
	}

	hasil, err := h.repo.Update(ctx, model.Student{
		ID: id, NIM: req.NIM, Name: req.Name, Grade: req.Grade, IsActive: req.IsActive,
	})
	if err != nil {
		return terjemahkanError(c, err, "gagal memperbarui mahasiswa")
	}

	return ok(c, "mahasiswa berhasil diganti seluruhnya", hasil)
}

// Patch menangani PATCH /api/v1/students/:id (ubah SEBAGIAN field saja)
func (h *StudentHandler) Patch(c *fiber.Ctx) error {
	ctx, cancel := reqCtx(c)
	defer cancel()

	id, valid := paramID(c)
	if !valid {
		return fail(c, fiber.StatusBadRequest, "id harus berupa angka positif")
	}

	var req model.StudentPatchRequest
	if err := c.BodyParser(&req); err != nil {
		return fail(c, fiber.StatusBadRequest, "body harus berupa JSON yang valid")
	}

	if req.NIM == nil && req.Name == nil && req.Grade == nil && req.IsActive == nil {
		return fail(c, fiber.StatusBadRequest, "tidak ada field yang dikirim untuk diubah")
	}

	// PATCH = baca dulu data saat ini, ubah seperlunya, lalu simpan kembali
	// lewat Update yang sama dengan PUT. Perbedaan PUT dan PATCH diputuskan
	// di lapisan handler ini, bukan di lapisan repository/penyimpanan.
	saatIni, err := h.repo.FindByID(ctx, id)
	if err != nil {
		return terjemahkanError(c, err, "gagal mengambil data mahasiswa")
	}

	if req.NIM != nil {
		nim := strings.TrimSpace(*req.NIM)
		if nim == "" {
			return failValidation(c, map[string]string{"nim": "tidak boleh kosong jika dikirim"})
		}
		saatIni.NIM = nim
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return failValidation(c, map[string]string{"name": "tidak boleh kosong jika dikirim"})
		}
		saatIni.Name = name
	}
	if req.Grade != nil {
		if *req.Grade < 0 || *req.Grade > 4 {
			return failValidation(c, map[string]string{"grade": "harus di antara 0 dan 4"})
		}
		saatIni.Grade = *req.Grade
	}
	if req.IsActive != nil {
		saatIni.IsActive = *req.IsActive
	}

	hasil, err := h.repo.Update(ctx, saatIni)
	if err != nil {
		return terjemahkanError(c, err, "gagal memperbarui mahasiswa")
	}

	return ok(c, "mahasiswa berhasil diperbarui sebagian", hasil)
}

// Delete menangani DELETE /api/v1/students/:id
func (h *StudentHandler) Delete(c *fiber.Ctx) error {
	ctx, cancel := reqCtx(c)
	defer cancel()

	id, valid := paramID(c)
	if !valid {
		return fail(c, fiber.StatusBadRequest, "id harus berupa angka positif")
	}

	if err := h.repo.Delete(ctx, id); err != nil {
		return terjemahkanError(c, err, "gagal menghapus mahasiswa")
	}

	return noContent(c)
}
