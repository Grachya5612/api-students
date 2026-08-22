package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// GetAllStudents menangani GET /api/v1/students
// Mendukung query string:
//   - page, limit      : paginasi (limit dibatasi maksimal MaxLimit)
//   - search            : cari nama mahasiswa (case-insensitive, substring)
//   - sort, order       : urutkan hasil (field HARUS ada di sortWhitelist)
//   - is_active         : filter status aktif (true/false)
//   - grade_min/grade_max: filter rentang nilai
func GetAllStudents(c *fiber.Ctx) error {
	// --- page & limit ---
	page, err := strconv.Atoi(c.Query("page", strconv.Itoa(DefaultPage)))
	if err != nil || page < 1 {
		page = DefaultPage
	}
	limit, err := strconv.Atoi(c.Query("limit", strconv.Itoa(DefaultLimit)))
	if err != nil || limit < 1 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	// --- search ---
	search := strings.TrimSpace(c.Query("search", ""))

	// --- sort & order (validasi lewat whitelist) ---
	sortField := c.Query("sort", "id")
	if !sortWhitelist[sortField] {
		return ErrorResponse(c, fiber.StatusBadRequest,
			"Field sort tidak valid. Field yang boleh diurutkan: id, nim, name, grade")
	}
	order := strings.ToLower(c.Query("order", "asc"))
	if order != "asc" && order != "desc" {
		return ErrorResponse(c, fiber.StatusBadRequest, "Field order harus 'asc' atau 'desc'")
	}

	// --- filter is_active (opsional) ---
	var isActiveFilter *bool
	if raw := c.Query("is_active", ""); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return ErrorResponse(c, fiber.StatusBadRequest, "Nilai is_active harus true atau false")
		}
		isActiveFilter = &parsed
	}

	// --- filter rentang grade (opsional) ---
	var gradeMin, gradeMax *float64
	if raw := c.Query("grade_min", ""); raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return ErrorResponse(c, fiber.StatusBadRequest, "Nilai grade_min harus berupa angka")
		}
		gradeMin = &parsed
	}
	if raw := c.Query("grade_max", ""); raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return ErrorResponse(c, fiber.StatusBadRequest, "Nilai grade_max harus berupa angka")
		}
		gradeMax = &parsed
	}

	mu.Lock()
	defer mu.Unlock()

	// Urutan proses: filter dulu (search, is_active, grade range), baru sort, baru paginasi.
	result := filterBySearch(students, search)
	result = filterByIsActive(result, isActiveFilter)
	result = filterByGradeRange(result, gradeMin, gradeMax)

	sortStudents(result, sortField, order)

	pagedData, meta := paginate(result, page, limit)

	return SuccessResponseWithMeta(c, fiber.StatusOK, "Berhasil mengambil daftar mahasiswa", pagedData, meta)
}

// GetStudentByID menangani GET /students/:id
// Mengembalikan satu data mahasiswa berdasarkan ID di URL.
func GetStudentByID(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return ErrorResponse(c, fiber.StatusBadRequest, "ID harus berupa angka")
	}

	mu.Lock()
	defer mu.Unlock()

	idx := findStudentIndexByID(id)
	if idx == -1 {
		return ErrorResponse(c, fiber.StatusNotFound, "Mahasiswa dengan ID tersebut tidak ditemukan")
	}

	return SuccessResponse(c, fiber.StatusOK, "Berhasil mengambil data mahasiswa", students[idx])
}

// CreateStudent menangani POST /api/v1/students
// Membuat data mahasiswa baru dari StudentCreateRequest.
func CreateStudent(c *fiber.Ctx) error {
	if !requireJSONContentType(c) {
		return ErrorResponse(c, fiber.StatusUnsupportedMediaType, "Content-Type harus application/json")
	}

	var req StudentCreateRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, fiber.StatusBadRequest, "Body bukan JSON yang sah")
	}

	if errs := validateStudentFields(req.NIM, req.Name, req.Grade); len(errs) > 0 {
		return ValidationErrorResponse(c, errs)
	}

	mu.Lock()
	defer mu.Unlock()

	if isNIMTaken(req.NIM, 0) {
		return ErrorResponse(c, fiber.StatusConflict, "NIM sudah terdaftar pada mahasiswa lain")
	}

	newStudent := Student{
		ID:       nextID,
		NIM:      req.NIM,
		Name:     req.Name,
		Grade:    req.Grade,
		IsActive: req.IsActive,
	}
	nextID++

	students = append(students, newStudent)

	// Header Location menunjuk ke alamat resource yang baru dibuat,
	// sesuai konvensi REST untuk response 201 Created.
	c.Set("Location", fmt.Sprintf("/api/v1/students/%d", newStudent.ID))

	return SuccessResponse(c, fiber.StatusCreated, "Mahasiswa baru berhasil ditambahkan", newStudent)
}

// UpdateStudent menangani PUT /api/v1/students/:id
// Mengganti SELURUH field data mahasiswa (full replace) dari StudentUpdateRequest.
func UpdateStudent(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return ErrorResponse(c, fiber.StatusBadRequest, "ID harus berupa angka")
	}

	if !requireJSONContentType(c) {
		return ErrorResponse(c, fiber.StatusUnsupportedMediaType, "Content-Type harus application/json")
	}

	var req StudentUpdateRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, fiber.StatusBadRequest, "Body bukan JSON yang sah")
	}

	if errs := validateStudentFields(req.NIM, req.Name, req.Grade); len(errs) > 0 {
		return ValidationErrorResponse(c, errs)
	}

	mu.Lock()
	defer mu.Unlock()

	idx := findStudentIndexByID(id)
	if idx == -1 {
		return ErrorResponse(c, fiber.StatusNotFound, "Mahasiswa dengan ID tersebut tidak ditemukan")
	}

	if isNIMTaken(req.NIM, id) {
		return ErrorResponse(c, fiber.StatusConflict, "NIM sudah terdaftar pada mahasiswa lain")
	}

	students[idx].NIM = req.NIM
	students[idx].Name = req.Name
	students[idx].Grade = req.Grade
	students[idx].IsActive = req.IsActive

	return SuccessResponse(c, fiber.StatusOK, "Data mahasiswa berhasil diperbarui", students[idx])
}

// PatchStudent menangani PATCH /api/v1/students/:id
// Memperbarui SEBAGIAN field saja, sesuai yang dikirim di StudentPatchRequest.
func PatchStudent(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return ErrorResponse(c, fiber.StatusBadRequest, "ID harus berupa angka")
	}

	if !requireJSONContentType(c) {
		return ErrorResponse(c, fiber.StatusUnsupportedMediaType, "Content-Type harus application/json")
	}

	var req StudentPatchRequest
	if err := c.BodyParser(&req); err != nil {
		return ErrorResponse(c, fiber.StatusBadRequest, "Body bukan JSON yang sah")
	}

	if errs := validatePatchFields(req); len(errs) > 0 {
		return ValidationErrorResponse(c, errs)
	}

	mu.Lock()
	defer mu.Unlock()

	idx := findStudentIndexByID(id)
	if idx == -1 {
		return ErrorResponse(c, fiber.StatusNotFound, "Mahasiswa dengan ID tersebut tidak ditemukan")
	}

	// Hanya field yang tidak nil (artinya benar-benar dikirim client) yang diperbarui
	if req.NIM != nil {
		if isNIMTaken(*req.NIM, id) {
			return ErrorResponse(c, fiber.StatusConflict, "NIM sudah terdaftar pada mahasiswa lain")
		}
		students[idx].NIM = *req.NIM
	}
	if req.Name != nil {
		students[idx].Name = *req.Name
	}
	if req.Grade != nil {
		students[idx].Grade = *req.Grade
	}
	if req.IsActive != nil {
		students[idx].IsActive = *req.IsActive
	}

	return SuccessResponse(c, fiber.StatusOK, "Data mahasiswa berhasil diperbarui sebagian", students[idx])
}

// DeleteStudent menangani DELETE /api/v1/students/:id
// Sesuai ketentuan: sukses hapus mengembalikan 204 No Content TANPA body.
// Kalau gagal (404), tetap pakai amplop Response seperti endpoint lain.
func DeleteStudent(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return ErrorResponse(c, fiber.StatusBadRequest, "ID harus berupa angka")
	}

	mu.Lock()
	defer mu.Unlock()

	idx := findStudentIndexByID(id)
	if idx == -1 {
		return ErrorResponse(c, fiber.StatusNotFound, "Mahasiswa dengan ID tersebut tidak ditemukan")
	}

	students = append(students[:idx], students[idx+1:]...)

	return c.SendStatus(fiber.StatusNoContent)
}