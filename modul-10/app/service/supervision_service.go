package service

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"api-students/app/model"
	"api-students/app/repository"
	"api-students/helper"
)

type SupervisionService struct {
	repo repository.SupervisionRepository
	tx   *repository.TxManager
}

func NewSupervisionService(repo repository.SupervisionRepository, tx *repository.TxManager) *SupervisionService {
	return &SupervisionService{repo: repo, tx: tx}
}

// ---------- POST /supervisors/:id/assign ----------

func (s *SupervisionService) Assign(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	supervisorID, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id harus berupa angka positif")
	}

	var req model.AssignRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}
	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	var updated model.Supervisor
	err := s.tx.WithTx(ctx, func(tx repository.DBTX) error {
		repo := s.repo.WithTx(tx)

		// 1. Kunci mahasiswa. Assign dan switch untuk mahasiswa yang sama
		// tidak bisa berjalan bersamaan.
		if err := repo.LockStudent(ctx, req.StudentID); err != nil {
			return err
		}

		// 2. Pastikan mahasiswa belum punya pembimbing.
		_, err := repo.FindSupervisorOf(ctx, req.StudentID)
		if err == nil {
			return repository.ErrAlreadyAssigned
		}
		if !errors.Is(err, repository.ErrNotAssigned) {
			return err
		}

		// 3. Ambil satu slot secara atomik.
		updated, err = repo.IncrementSupervised(ctx, supervisorID)
		if err != nil {
			return err
		}

		// 4. Catat penetapan. Bila gagal, slot dari langkah 3 ikut dibatalkan.
		return repo.InsertSupervision(ctx, req.StudentID, supervisorID)
	})
	if err != nil {
		return translateSupervisionError(err)
	}

	return helper.Created(c, "pembimbing berhasil ditetapkan", updated,
		"/api/v1/supervisors/"+strconv.Itoa(supervisorID))
}

// ---------- POST /supervisions/switch ----------
//
// Mengganti pembimbing secara atomik. Bila pembimbing baru penuh, error
// membatalkan seluruh transaction, sehingga mahasiswa tetap di pembimbing lama.

func (s *SupervisionService) Switch(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	var req model.SwitchRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}
	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	var updated model.Supervisor
	err := s.tx.WithTx(ctx, func(tx repository.DBTX) error {
		repo := s.repo.WithTx(tx)

		// 1. Kunci mahasiswa lebih dulu, selalu sebagai langkah pertama.
		if err := repo.LockStudent(ctx, req.StudentID); err != nil {
			return err
		}

		// 2. Baca pembimbing lama SETELAH kunci mahasiswa diambil.
		// Sebelum kunci, nilai ini bisa sudah berubah oleh request lain.
		currentID, err := repo.FindSupervisorOf(ctx, req.StudentID)
		if err != nil {
			return err
		}
		if currentID == req.ToSupervisorID {
			return repository.ErrSameSupervisor
		}

		// 3. Kunci kedua pembimbing dalam urutan id menaik.
		if err := repo.LockSupervisors(ctx, currentID, req.ToSupervisorID); err != nil {
			return err
		}

		// 4. Lepas dari pembimbing lama.
		if err := repo.DeleteSupervision(ctx, req.StudentID); err != nil {
			return err
		}
		if _, err := repo.DecrementSupervised(ctx, currentID); err != nil {
			return err
		}

		// 5. Masuk ke pembimbing baru. Bila penuh, error di sini
		// membatalkan langkah 4 juga.
		updated, err = repo.IncrementSupervised(ctx, req.ToSupervisorID)
		if err != nil {
			return err
		}
		return repo.InsertSupervision(ctx, req.StudentID, req.ToSupervisorID)
	})
	if err != nil {
		return translateSupervisionError(err)
	}

	return helper.Success(c, fiber.StatusOK, "pembimbing berhasil diganti", updated)
}

// ---------- PATCH /supervisors/:id ----------
// Optimistic locking. Satu pernyataan UPDATE, jadi tidak memakai WithTx.

func (s *SupervisionService) UpdateQuota(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	id, valid := helper.ParamID(c)
	if !valid {
		return helper.BadRequest("id harus berupa angka positif")
	}

	var req model.UpdateQuotaRequest
	if err := c.BodyParser(&req); err != nil {
		return helper.BadRequest("body harus berupa JSON yang valid")
	}
	if errs := helper.ValidateStruct(req); errs != nil {
		return helper.Validation(errs)
	}

	updated, err := s.repo.UpdateSupervisorQuota(ctx, id, req.Quota, req.Version)
	if err != nil {
		return translateSupervisionError(err)
	}

	return helper.Success(c, fiber.StatusOK, "kuota pembimbing berhasil diubah", updated)
}

// translateSupervisionError memetakan kegagalan aturan bisnis ke 409.
// Kegagalan yang tidak dikenali menjadi 500.
func translateSupervisionError(err error) error {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return helper.NotFound("data pembimbing atau mahasiswa tidak ditemukan")

	case errors.Is(err, repository.ErrSupervisorFull):
		return helper.NewError(fiber.StatusConflict, "SUPERVISOR_FULL",
			"kuota pembimbing sudah penuh")

	case errors.Is(err, repository.ErrAlreadyAssigned):
		return helper.NewError(fiber.StatusConflict, "ALREADY_ASSIGNED",
			"mahasiswa sudah memiliki pembimbing")

	case errors.Is(err, repository.ErrNotAssigned):
		return helper.NewError(fiber.StatusConflict, "NOT_ASSIGNED",
			"mahasiswa belum memiliki pembimbing")

	case errors.Is(err, repository.ErrSameSupervisor):
		return helper.NewError(fiber.StatusConflict, "SAME_SUPERVISOR",
			"pembimbing baru sama dengan pembimbing saat ini")

	case errors.Is(err, repository.ErrVersionConflict):
		return helper.NewError(fiber.StatusConflict, "VERSION_CONFLICT",
			"data pembimbing sudah diubah pihak lain, muat ulang lalu coba lagi")

	case errors.Is(err, repository.ErrQuotaBelowFilled):
		return helper.NewError(fiber.StatusConflict, "QUOTA_BELOW_SUPERVISED",
			"kuota tidak boleh lebih kecil dari jumlah mahasiswa bimbingan")

	default:
		return helper.Internal(err)
	}
}
