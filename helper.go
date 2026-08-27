package main

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"api-students/app/model"
)

// ===================== KONSTANTA QUERY STRING =====================
const (
	DefaultPage  = 1
	DefaultLimit = 10
	// MaxLimit membatasi berapa banyak data yang boleh diminta client
	// sekaligus. Tanpa batas ini, client bisa minta limit=1000000 dan
	// memaksa server + database mengirim payload raksasa dalam satu
	// request -- boros memori/bandwidth dan berpotensi jadi vektor DoS.
	MaxLimit = 100
)

var sortWhitelistForValidation = map[string]bool{
	"id": true, "nim": true, "name": true, "grade": true,
}

// ===================== CONTEXT & TIMEOUT =====================

// reqCtx memberi batas waktu untuk setiap operasi basis data.
// Tanpa batas waktu, satu query yang menggantung dapat menahan koneksi
// selamanya, dan lama-lama menghabiskan seluruh isi connection pool.
func reqCtx(c *fiber.Ctx) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.UserContext(), 5*time.Second)
}

// ===================== RESPONSE HELPER =====================

func ok(c *fiber.Ctx, message string, data interface{}) error {
	return c.Status(fiber.StatusOK).JSON(model.Response{
		Success: true, Message: message, Data: data,
	})
}

func okList(c *fiber.Ctx, message string, data interface{}, meta *model.Meta) error {
	return c.Status(fiber.StatusOK).JSON(model.Response{
		Success: true, Message: message, Data: data, Meta: meta,
	})
}

func created(c *fiber.Ctx, message string, data interface{}, location string) error {
	c.Set("Location", location)
	return c.Status(fiber.StatusCreated).JSON(model.Response{
		Success: true, Message: message, Data: data,
	})
}

func noContent(c *fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNoContent)
}

func fail(c *fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(model.Response{
		Success: false, Message: message,
	})
}

// failValidation mengembalikan 422 Unprocessable Entity dengan rincian
// kegagalan PER FIELD, supaya client tahu persis field mana yang salah.
func failValidation(c *fiber.Ctx, errs map[string]string) error {
	fieldErrors := make([]model.FieldError, 0, len(errs))
	for field, msg := range errs {
		fieldErrors = append(fieldErrors, model.FieldError{Field: field, Message: msg})
	}
	return c.Status(fiber.StatusUnprocessableEntity).JSON(model.Response{
		Success: false,
		Message: "Validasi gagal, periksa kembali field yang dikirim",
		Data:    fiber.Map{"errors": fieldErrors},
	})
}

// ===================== PARSING PARAM & QUERY =====================

// paramID mengambil & memvalidasi :id dari URL. Mengembalikan valid=false
// kalau bukan angka positif.
func paramID(c *fiber.Ctx) (int, bool) {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// parseListQuery membaca & menormalisasi seluruh query string pada
// GET /students menjadi model.ListQuery yang siap dipakai repository.
func parseListQuery(c *fiber.Ctx) model.ListQuery {
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

	sort := c.Query("sort", "id")
	if !sortWhitelistForValidation[sort] {
		sort = "id"
	}

	order := strings.ToLower(c.Query("order", "asc"))
	if order != "asc" && order != "desc" {
		order = "asc"
	}

	var isActive *bool
	if v := c.Query("is_active", ""); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			isActive = &b
		}
	}

	var gradeMin, gradeMax *float64
	if v := c.Query("grade_min", ""); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			gradeMin = &f
		}
	}
	if v := c.Query("grade_max", ""); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			gradeMax = &f
		}
	}

	return model.ListQuery{
		Page: page, Limit: limit,
		Search: strings.TrimSpace(c.Query("search", "")),
		Sort:   sort, Order: order,
		IsActive: isActive, GradeMin: gradeMin, GradeMax: gradeMax,
	}
}

// ===================== MIDDLEWARE =====================

// requireJSON memastikan request dengan body (POST/PUT/PATCH) memang
// mengirim Content-Type application/json. Kalau tidak, langsung ditolak
// 415 sebelum masuk ke handler sama sekali.
func requireJSON(c *fiber.Ctx) error {
	method := c.Method()
	if method == fiber.MethodPost || method == fiber.MethodPut || method == fiber.MethodPatch {
		ct := strings.ToLower(c.Get("Content-Type"))
		if !strings.HasPrefix(ct, "application/json") {
			return fail(c, fiber.StatusUnsupportedMediaType, "Content-Type harus application/json")
		}
	}
	return c.Next()
}
