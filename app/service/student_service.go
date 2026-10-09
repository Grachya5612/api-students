package service

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"api-students/app/model"
	"api-students/app/repository"
	"api-students/helper"
)

// StudentService menangani CRUD student dan aturan ownership.
type StudentService struct {
	repo  repository.StudentRepository
	perms *helper.PermissionSet
}

// NewStudentService menerima interface repository.
func NewStudentService(
	repo repository.StudentRepository,
	perms *helper.PermissionSet,
) *StudentService {
	return &StudentService{
		repo:  repo,
		perms: perms,
	}
}

// ---------- GET /students ----------

func (s *StudentService) List(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	// Parse cursor query (bukan offset pagination)
	cq, err := helper.ParseCursorQuery(c)
	if err != nil {
		return err
	}

	students, meta, err := s.repo.FindAfterCursor(ctx, cq)
	if err != nil {
		return helper.Internal(err)
	}

	return helper.SuccessCursor(
		c,
		"daftar student berhasil diambil",
		students,
		&meta,
	)
}

// ---------- GET /students/:id ----------

func (s *StudentService) Get(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	current, ok := helper.CurrentUser(c)
	if !ok {
		return helper.Unauthorized("belum terautentikasi")
	}

	id, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id harus berupa angka positif")
	}

	student, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return translateStudentError(err)
	}

	// Owner boleh membaca datanya sendiri.
	// User lain membutuhkan student:read:any.
	if !CanAccessStudent(current, student.OwnerID, s.perms, "student:read:any") {
		return helper.Forbidden("tidak berhak mengakses data student ini")
	}

	return helper.Success(
		c,
		fiber.StatusOK,
		"student ditemukan",
		student,
	)
}

// ---------- POST /students ----------

func (s *StudentService) Create(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	current, ok := helper.CurrentUser(c)
	if !ok {
		return helper.Unauthorized("belum terautentikasi")
	}

	var req model.StudentCreateRequest

	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}

	req.NIM = strings.TrimSpace(req.NIM)
	req.Name = strings.TrimSpace(req.Name)

	// Validasi deklaratif - tag validate pada struct
	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	// Owner ditentukan oleh server berdasarkan identitas JWT.
	// OwnerID tidak pernah diambil dari request body.
	ownerID := current.UserID

	student, err := s.repo.Create(ctx, model.Student{
		NIM:      req.NIM,
		Name:     req.Name,
		Grade:    req.Grade,
		IsActive: req.IsActive,
		OwnerID:  ownerID,
	})

	if err != nil {
		return translateStudentError(err)
	}

	return helper.Created(
		c,
		"student berhasil dibuat",
		student,
		"/api/v1/students/"+strconv.Itoa(student.ID),
	)
}

// ---------- PUT /students/:id ----------

func (s *StudentService) Replace(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	current, ok := helper.CurrentUser(c)
	if !ok {
		return helper.Unauthorized("belum terautentikasi")
	}

	id, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id harus berupa angka positif")
	}

	// Ambil data lama untuk mengetahui owner.
	student, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return translateStudentError(err)
	}

	if !CanAccessStudent(
		current,
		student.OwnerID,
		s.perms,
		"student:update:any",
	) {
		return helper.Forbidden("tidak berhak mengubah data student ini")
	}

	var req model.StudentUpdateRequest

	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}

	// Validasi deklaratif
	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	// OwnerID tetap memakai owner lama.
	// PUT tidak boleh memindahkan kepemilikan student.
	updated, err := s.repo.Update(ctx, model.Student{
		ID:       id,
		NIM:      strings.TrimSpace(req.NIM),
		Name:     strings.TrimSpace(req.Name),
		Grade:    req.Grade,
		IsActive: req.IsActive,
		OwnerID:  student.OwnerID,
	})

	if err != nil {
		return translateStudentError(err)
	}

	return helper.Success(
		c,
		fiber.StatusOK,
		"student berhasil diganti seluruhnya",
		updated,
	)
}

// ---------- PATCH /students/:id ----------

func (s *StudentService) Patch(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	current, ok := helper.CurrentUser(c)
	if !ok {
		return helper.Unauthorized("belum terautentikasi")
	}

	id, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id harus berupa angka positif")
	}

	// Ambil data lama terlebih dahulu untuk pemeriksaan ownership.
	student, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return translateStudentError(err)
	}

	if !CanAccessStudent(
		current,
		student.OwnerID,
		s.perms,
		"student:update:any",
	) {
		return helper.Forbidden("tidak berhak mengubah data student ini")
	}

	var req model.StudentPatchRequest

	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}

	// Validasi deklaratif
	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	if IsEmptyStudentPatch(req) {
		return helper.BadRequest("tidak ada field yang diubah")
	}

	// ApplyStudentPatch tidak lagi mengembalikan error - hanya menggabung
	updated := ApplyStudentPatch(student, req)

	// ApplyStudentPatch tidak pernah mengubah OwnerID.
	result, err := s.repo.Update(ctx, updated)
	if err != nil {
		return translateStudentError(err)
	}

	return helper.Success(
		c,
		fiber.StatusOK,
		"student berhasil diperbarui sebagian",
		result,
	)
}

// ---------- DELETE /students/:id ----------

func (s *StudentService) Delete(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	if _, ok := helper.CurrentUser(c); !ok {
		return helper.Unauthorized("belum terautentikasi")
	}

	id, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id harus berupa angka positif")
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return translateStudentError(err)
	}

	return helper.NoContent(c)
}

// translateStudentError mengubah error milik repository menjadi AppError.
// Tidak ada fiber.Ctx - fungsi ini hanya menerjemahkan satu jenis error
// menjadi jenis lain, tidak tahu apa pun tentang HTTP.
func translateStudentError(err error) error {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return helper.NotFound("student tidak ditemukan")

	case errors.Is(err, repository.ErrDuplicate):
		return helper.Conflict("nim sudah dipakai")

	default:
		return helper.Internal(err)
	}
}
