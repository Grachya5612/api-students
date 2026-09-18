package service

import (
	"errors"
	"strconv"
	"strings"
	"fmt"

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

	q := helper.ParseListQuery(c)

	students, total, err := s.repo.FindAll(ctx, q)
	if err != nil {
		fmt.Printf("ERROR STUDENT LIST: %v\n", err)
		return helper.Fail(
			c,
			fiber.StatusInternalServerError,
			"gagal mengambil data student",
		)
	}

	return helper.SuccessList(
		c,
		"daftar student berhasil diambil",
		students,
		&model.Meta{
			Page:       q.Page,
			Limit:      q.Limit,
			Total:      total,
			TotalPages: CountTotalPages(total, q.Limit),
		},
	)
}

// ---------- GET /students/:id ----------

func (s *StudentService) Get(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	current, ok := helper.CurrentUser(c)
	if !ok {
		return helper.Fail(
			c,
			fiber.StatusUnauthorized,
			"belum terautentikasi",
		)
	}

	id, valid := helper.ParamID(c)
	if !valid {
		return helper.Fail(
			c,
			fiber.StatusBadRequest,
			"id harus berupa angka positif",
		)
	}

	student, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return translateStudentError(
			c,
			err,
			"gagal mengambil data student",
		)
	}

	// Owner boleh membaca datanya sendiri.
	// User lain membutuhkan student:read:any.
	if !CanAccessStudent(current, student.OwnerID, s.perms, "student:read:any") {
		return helper.Fail(
			c,
			fiber.StatusForbidden,
			"tidak berhak mengakses data student ini",
		)
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
		return helper.Fail(
			c,
			fiber.StatusUnauthorized,
			"belum terautentikasi",
		)
	}

	var req model.StudentCreateRequest

	if err := c.BodyParser(&req); err != nil {
		return helper.Fail(
			c,
			fiber.StatusBadRequest,
			"body harus berupa JSON yang valid",
		)
	}

	req.NIM = strings.TrimSpace(req.NIM)
	req.Name = strings.TrimSpace(req.Name)

	if errs := ValidateStudentCreate(req); len(errs) > 0 {
		return helper.FailValidation(c, errs)
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
		return translateStudentError(
			c,
			err,
			"gagal menyimpan student",
		)
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
		return helper.Fail(
			c,
			fiber.StatusUnauthorized,
			"belum terautentikasi",
		)
	}

	id, valid := helper.ParamID(c)
	if !valid {
		return helper.Fail(
			c,
			fiber.StatusBadRequest,
			"id harus berupa angka positif",
		)
	}

	// Ambil data lama untuk mengetahui owner.
	student, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return translateStudentError(
			c,
			err,
			"gagal mengambil data student",
		)
	}

	if !CanAccessStudent(
		current,
		student.OwnerID,
		s.perms,
		"student:update:any",
	) {

		return helper.Fail(
			c,
			fiber.StatusForbidden,
			"tidak berhak mengubah data student ini",
		)
	}

	var req model.StudentUpdateRequest

	if err := c.BodyParser(&req); err != nil {
		return helper.Fail(
			c,
			fiber.StatusBadRequest,
			"body harus berupa JSON yang valid",
		)
	}

	if errs := ValidateStudentReplace(req); len(errs) > 0 {
		return helper.FailValidation(c, errs)
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
		return translateStudentError(
			c,
			err,
			"gagal memperbarui student",
		)
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
		return helper.Fail(
			c,
			fiber.StatusUnauthorized,
			"belum terautentikasi",
		)
	}

	id, valid := helper.ParamID(c)
	if !valid {
		return helper.Fail(
			c,
			fiber.StatusBadRequest,
			"id harus berupa angka positif",
		)
	}

	// Ambil data lama terlebih dahulu untuk pemeriksaan ownership.
	student, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return translateStudentError(
			c,
			err,
			"gagal mengambil data student",
		)
	}

	if !CanAccessStudent(
		current,
		student.OwnerID,
		s.perms,
		"student:update:any",
	) {
		return helper.Fail(
			c,
			fiber.StatusForbidden,
			"tidak berhak mengubah data student ini",
		)
	}

	var req model.StudentPatchRequest

	if err := c.BodyParser(&req); err != nil {
		return helper.Fail(
			c,
			fiber.StatusBadRequest,
			"body harus berupa JSON yang valid",
		)
	}

	if IsEmptyStudentPatch(req) {
		return helper.Fail(
			c,
			fiber.StatusBadRequest,
			"tidak ada field yang diubah",
		)
	}

	updated, errs := ApplyStudentPatch(student, req)
	if len(errs) > 0 {
		return helper.FailValidation(c, errs)
	}

	// ApplyStudentPatch tidak pernah mengubah OwnerID.
	result, err := s.repo.Update(ctx, updated)
	if err != nil {
		return translateStudentError(
			c,
			err,
			"gagal memperbarui student",
		)
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
		return helper.Fail(
			c,
			fiber.StatusUnauthorized,
			"belum terautentikasi",
		)
	}

	id, valid := helper.ParamID(c)
	if !valid {
		return helper.Fail(
			c,
			fiber.StatusBadRequest,
			"id harus berupa angka positif",
		)
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return translateStudentError(
			c,
			err,
			"gagal menghapus student",
		)
	}

	return helper.NoContent(c)
}

// translateStudentError memetakan error repository ke HTTP response.
func translateStudentError(
	c *fiber.Ctx,
	err error,
	generalMessage string,
) error {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return helper.Fail(
			c,
			fiber.StatusNotFound,
			"student tidak ditemukan",
		)

	case errors.Is(err, repository.ErrDuplicate):
		return helper.Fail(
			c,
			fiber.StatusConflict,
			"nim sudah dipakai",
		)

	default:
		return helper.Fail(
			c,
			fiber.StatusInternalServerError,
			generalMessage,
		)
	}
}
