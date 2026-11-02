package helper

import (
	"encoding/csv"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"api-students/app/model"
)

const (
	// FormatJSON adalah MIME type untuk JSON
	FormatJSON = fiber.MIMEApplicationJSON
	// FormatCSV adalah MIME type untuk CSV
	FormatCSV = "text/csv"
)

// Negotiate memilih format response berdasarkan header Accept.
// Jika tidak ada Accept header, gunakan format pertama dari offered.
// Jika client request format yang tidak tersedia, return NotAcceptable error.
func Negotiate(c *fiber.Ctx, offered ...string) (string, error) {
	accept := strings.TrimSpace(c.Get(fiber.HeaderAccept))

	// Tidak menyebut Accept sama sekali berarti "terserah server"
	if accept == "" {
		return offered[0], nil
	}

	// Jika Accept header berisi wildcard *, terima format pertama
	if accept == "*/*" {
		return offered[0], nil
	}

	// Cek apakah client request format ada di offered list
	// Parse Accept header - ambil bagian sebelum semicolon (quality factor)
	acceptParts := strings.Split(accept, ",")
	for _, part := range acceptParts {
		part = strings.TrimSpace(part)
		// Ambil MIME type (sebelum ;)
		if idx := strings.Index(part, ";"); idx != -1 {
			part = part[:idx]
		}
		part = strings.TrimSpace(part)

		// Cek apakah part ada di offered
		for _, off := range offered {
			if strings.EqualFold(part, off) {
				return part, nil
			}
		}
	}

	// Jika tidak ada yang match, return 406
	return "", NotAcceptable(
		"format yang diminta tidak tersedia, pilih salah satu dari: " +
			strings.Join(offered, ", "))
}

// WriteStudentsCSV menuliskan daftar student sebagai CSV.
// Set Content-Type header dan tulis data sebagai CSV.
func WriteStudentsCSV(c *fiber.Ctx, students []model.Student) error {
	c.Set(fiber.HeaderContentType, FormatCSV+"; charset=utf-8")
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="students.csv"`)

	// Gunakan strings.Builder untuk buffer output
	var buffer strings.Builder
	writer := csv.NewWriter(&buffer)

	// Header baris pertama
	header := []string{"id", "nim", "name", "grade", "is_active", "created_at"}
	if err := writer.Write(header); err != nil {
		return Internal(err)
	}

	// Data rows
	for _, st := range students {
		row := []string{
			strconv.Itoa(st.ID),
			st.NIM,
			st.Name,
			strconv.FormatFloat(st.Grade, 'f', 2, 64),
			strconv.FormatBool(st.IsActive),
			st.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		}
		if err := writer.Write(row); err != nil {
			return Internal(err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return Internal(err)
	}

	return c.SendString(buffer.String())
}