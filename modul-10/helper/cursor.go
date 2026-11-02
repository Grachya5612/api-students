package helper

import (
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"api-students/app/model"
)

// EncodeCursor mengubah penanda menjadi string yang aman di URL.
// Menggunakan base64 encoding agar bisa dikirim di query string.
func EncodeCursor(createdAt time.Time, id int) string {
	raw := strconv.FormatInt(createdAt.UTC().UnixNano(), 10) + "|" + strconv.Itoa(id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeCursor membaca kembali penanda dari string.
// Mengembalikan error jika format tidak valid.
func DecodeCursor(encoded string) (model.Cursor, error) {
	if strings.TrimSpace(encoded) == "" {
		return model.Cursor{}, BadRequest("cursor tidak valid")
	}

	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return model.Cursor{}, BadRequest("cursor tidak valid")
	}

	parts := strings.Split(string(decoded), "|")
	if len(parts) != 2 {
		return model.Cursor{}, BadRequest("cursor tidak valid")
	}

	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return model.Cursor{}, BadRequest("cursor tidak valid")
	}

	id, err := strconv.Atoi(parts[1])
	if err != nil || id < 1 {
		return model.Cursor{}, BadRequest("cursor tidak valid")
	}

	return model.Cursor{
		CreatedAt: time.Unix(0, nanos).UTC(),
		ID:        id,
	}, nil
}

// ParseCursorQuery membaca query string untuk cursor pagination.
// Extract: cursor (position), limit (page size), search (filter), is_active (filter)
func ParseCursorQuery(c *fiber.Ctx) (model.CursorQuery, error) {
	q := model.CursorQuery{
		Limit:  c.QueryInt("limit", 10),
		Search: strings.TrimSpace(c.Query("search")),
	}

	// Validasi limit
	if q.Limit < 1 {
		q.Limit = 10
	}
	if q.Limit > 100 {
		q.Limit = 100
	}

	// Parse cursor jika ada
	if cursorStr := c.Query("cursor"); cursorStr != "" {
		cursor, err := DecodeCursor(cursorStr)
		if err != nil {
			return model.CursorQuery{}, err
		}
		q.After = &cursor
	}

	// Parse is_active filter jika ada
	if raw := c.Query("is_active"); raw != "" {
		if v, err := strconv.ParseBool(raw); err == nil {
			q.IsActive = &v
		}
	}

	return q, nil
}