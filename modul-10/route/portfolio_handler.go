package route

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"

	"api-students/app/model"
	"api-students/app/repository"
	"api-students/app/service"
	"api-students/helper"
)

type portfolioHandler struct {
	svc *service.PortfolioService
}

func newPortfolioHandler(svc *service.PortfolioService) *portfolioHandler {
	return &portfolioHandler{svc: svc}
}

type portfolioBody struct {
	Headline string `json:"headline"`
	Bio      string `json:"bio"`
}

func parseStudentID(c *fiber.Ctx) (int, error) {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil || id <= 0 {
		return 0, fiber.NewError(fiber.StatusBadRequest, "id mahasiswa tidak valid")
	}
	return id, nil
}

// portfolioError menerjemahkan error domain menjadi status HTTP.
func portfolioError(err error) error {
	switch {
	case errors.Is(err, service.ErrStudentNotFound):
		return fiber.NewError(fiber.StatusNotFound, "mahasiswa tidak ditemukan")
	case errors.Is(err, repository.ErrPortfolioNotFound):
		return fiber.NewError(fiber.StatusNotFound, "portofolio belum dibuat")
	case errors.Is(err, repository.ErrProjectNotFound):
		return fiber.NewError(fiber.StatusNotFound, "proyek tidak ditemukan")
	case errors.Is(err, repository.ErrProjectLimit):
		return fiber.NewError(fiber.StatusConflict, "batas 20 proyek sudah tercapai")
	case errors.Is(err, repository.ErrInvalidProjectID):
		return fiber.NewError(fiber.StatusBadRequest, "id proyek tidak valid")
	case errors.Is(err, service.ErrInvalidInput):
		return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	}
	return err
}

// GET /students/:id/portfolio
func (h *portfolioHandler) get(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	id, err := parseStudentID(c)
	if err != nil {
		return err
	}
	p, err := h.svc.Get(ctx, id)
	if err != nil {
		return portfolioError(err)
	}
	return helper.Success(c, fiber.StatusOK, "portofolio ditemukan", p)
}

// PUT /students/:id/portfolio
func (h *portfolioHandler) upsert(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	id, err := parseStudentID(c)
	if err != nil {
		return err
	}
	var body portfolioBody
	if err := c.BodyParser(&body); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "body tidak valid")
	}
	if err := h.svc.Upsert(ctx, id, body.Headline, body.Bio); err != nil {
		return portfolioError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// POST /students/:id/portfolio/projects
func (h *portfolioHandler) addProject(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	id, err := parseStudentID(c)
	if err != nil {
		return err
	}
	var in model.ProjectInput
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "body tidak valid")
	}
	p, err := h.svc.AddProject(ctx, id, in)
	if err != nil {
		return portfolioError(err)
	}
	return helper.Success(c, fiber.StatusCreated, "proyek ditambahkan", p)
}

// DELETE /students/:id/portfolio/projects/:projectId
func (h *portfolioHandler) deleteProject(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	id, err := parseStudentID(c)
	if err != nil {
		return err
	}
	if err := h.svc.DeleteProject(ctx, id, c.Params("projectId")); err != nil {
		return portfolioError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// GET /portfolios/stats/technologies
func (h *portfolioHandler) techStats(c *fiber.Ctx) error {
	ctx, cancel := helper.RequestContext(c)
	defer cancel()

	stats, err := h.svc.TechnologyStats(ctx)
	if err != nil {
		return portfolioError(err)
	}
	return helper.Success(c, fiber.StatusOK, "statistik teknologi", stats)
}
