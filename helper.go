package main

import (
	"sort"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"
)

// ===================== KONSTANTA QUERY STRING =====================
const (
	DefaultPage  = 1
	DefaultLimit = 10

	// MaxLimit dibatasi di 100. Alasannya: kalau client boleh minta limit
	// sebebas-bebasnya (misal limit=100000), server bisa dipaksa memproses
	// & mengirim payload raksasa dalam satu request -- boros memori/bandwidth,
	// dan bisa disalahgunakan sebagai vektor DoS sederhana. 100 dipilih
	// sebagai titik tengah: cukup besar untuk kebutuhan export/laporan
	// singkat, tapi masih jauh dari ukuran yang bisa membebani server
	// in-memory sederhana seperti ini.
	MaxLimit = 100
)

// sortWhitelist adalah daftar putih field yang boleh dipakai untuk
// mengurutkan hasil. Field di luar daftar ini akan ditolak (400 Bad Request)
// supaya tidak ada yang bisa "menebak-nebak" nama field internal atau
// memicu error dari field yang tidak didukung.
var sortWhitelist = map[string]bool{
	"id":    true,
	"nim":   true,
	"name":  true,
	"grade": true,
}

// ===================== PENYIMPANAN DATA (IN-MEMORY) =====================
// Karena belum pakai database, data mahasiswa disimpan sementara di slice.
// Mutex dipakai supaya aman diakses dari beberapa request bersamaan (thread-safe).
var (
	students = []Student{
		{ID: 1, NIM: "2024001", Name: "Cia", Grade: 3.45, IsActive: true},
		{ID: 2, NIM: "2024002", Name: "Ghea", Grade: 3.20, IsActive: true},
		{ID: 3, NIM: "2024003", Name: "Syanda", Grade: 2.90, IsActive: false},
		{ID: 4, NIM: "2024004", Name: "Ika", Grade: 3.80, IsActive: true},
	}
	nextID = 5
	mu     sync.Mutex
)

// ===================== HELPER RESPONSE =====================
// SuccessResponse membungkus response sukses dalam amplop Response yang seragam.
func SuccessResponse(c *fiber.Ctx, statusCode int, message string, data interface{}) error {
	return c.Status(statusCode).JSON(Response{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// ErrorResponse membungkus response gagal dalam amplop Response yang SAMA
// dengan response sukses, hanya beda nilai Success & Data-nya.
func ErrorResponse(c *fiber.Ctx, statusCode int, message string) error {
	return c.Status(statusCode).JSON(Response{
		Success: false,
		Message: message,
		Data:    nil,
	})
}

// SuccessResponseWithMeta sama seperti SuccessResponse, tapi menyertakan
// info tambahan (misalnya paginasi) di field "meta".
func SuccessResponseWithMeta(c *fiber.Ctx, statusCode int, message string, data interface{}, meta interface{}) error {
	return c.Status(statusCode).JSON(Response{
		Success: true,
		Message: message,
		Data:    data,
		Meta:    meta,
	})
}

// ValidationErrorResponse mengembalikan 422 Unprocessable Entity beserta
// rincian kegagalan PER FIELD, supaya client tahu persis apa yang salah.
func ValidationErrorResponse(c *fiber.Ctx, errors []FieldError) error {
	return c.Status(fiber.StatusUnprocessableEntity).JSON(Response{
		Success: false,
		Message: "Validasi gagal, periksa kembali field yang dikirim",
		Data:    fiber.Map{"errors": errors},
	})
}

// requireJSONContentType mengecek header Content-Type request.
// Dipakai di endpoint yang menerima body (POST/PUT/PATCH) untuk
// memastikan client memang mengirim JSON, bukan format lain.
func requireJSONContentType(c *fiber.Ctx) bool {
	ct := strings.ToLower(c.Get("Content-Type"))
	return strings.HasPrefix(ct, "application/json")
}

// validateStudentFields memvalidasi field WAJIB (dipakai untuk POST & PUT,
// yang mengharuskan semua field terisi dan valid).
func validateStudentFields(nim, name string, grade float64) []FieldError {
	var errs []FieldError
	if strings.TrimSpace(nim) == "" {
		errs = append(errs, FieldError{Field: "nim", Message: "NIM wajib diisi"})
	}
	if strings.TrimSpace(name) == "" {
		errs = append(errs, FieldError{Field: "name", Message: "Name wajib diisi"})
	}
	if grade < 0 || grade > 4 {
		errs = append(errs, FieldError{Field: "grade", Message: "Grade harus di antara 0 dan 4"})
	}
	return errs
}

// validatePatchFields memvalidasi field OPSIONAL (dipakai untuk PATCH).
// Hanya field yang benar-benar dikirim (tidak nil) yang divalidasi.
func validatePatchFields(req StudentPatchRequest) []FieldError {
	var errs []FieldError
	if req.NIM != nil && strings.TrimSpace(*req.NIM) == "" {
		errs = append(errs, FieldError{Field: "nim", Message: "NIM tidak boleh kosong jika dikirim"})
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		errs = append(errs, FieldError{Field: "name", Message: "Name tidak boleh kosong jika dikirim"})
	}
	if req.Grade != nil && (*req.Grade < 0 || *req.Grade > 4) {
		errs = append(errs, FieldError{Field: "grade", Message: "Grade harus di antara 0 dan 4"})
	}
	return errs
}

// ===================== HELPER DATA =====================
// findStudentIndexByID mencari posisi (index) student di slice berdasarkan ID.
// Mengembalikan -1 kalau tidak ditemukan.
func findStudentIndexByID(id int) int {
	for i, s := range students {
		if s.ID == id {
			return i
		}
	}
	return -1
}

// isNIMTaken mengecek apakah NIM sudah dipakai student lain.
// excludeID dipakai saat update, supaya student itu sendiri tidak dianggap "bentrok".
func isNIMTaken(nim string, excludeID int) bool {
	for _, s := range students {
		if s.NIM == nim && s.ID != excludeID {
			return true
		}
	}
	return false
}

// filterBySearch menyaring student yang nama-nya MENGANDUNG kata kunci,
// tidak membedakan huruf besar/kecil (case-insensitive).
func filterBySearch(data []Student, search string) []Student {
	if search == "" {
		return data
	}
	search = strings.ToLower(search)
	result := make([]Student, 0, len(data))
	for _, s := range data {
		if strings.Contains(strings.ToLower(s.Name), search) {
			result = append(result, s)
		}
	}
	return result
}

// filterByIsActive menyaring student berdasarkan status aktif.
// activeFilter bernilai nil kalau query param "is_active" tidak dikirim
// sama sekali (artinya: tidak difilter, tampilkan semua).
func filterByIsActive(data []Student, activeFilter *bool) []Student {
	if activeFilter == nil {
		return data
	}
	result := make([]Student, 0, len(data))
	for _, s := range data {
		if s.IsActive == *activeFilter {
			result = append(result, s)
		}
	}
	return result
}

// filterByGradeRange menyaring student yang grade-nya berada di rentang
// [min, max]. min/max bernilai nil kalau query param terkait tidak dikirim.
func filterByGradeRange(data []Student, min, max *float64) []Student {
	if min == nil && max == nil {
		return data
	}
	result := make([]Student, 0, len(data))
	for _, s := range data {
		if min != nil && s.Grade < *min {
			continue
		}
		if max != nil && s.Grade > *max {
			continue
		}
		result = append(result, s)
	}
	return result
}

// sortStudents mengurutkan data berdasarkan field & arah yang diminta.
// Field WAJIB sudah divalidasi lewat sortWhitelist sebelum function ini dipanggil.
func sortStudents(data []Student, field string, order string) {
	desc := order == "desc"

	sort.Slice(data, func(i, j int) bool {
		var less bool
		switch field {
		case "id":
			less = data[i].ID < data[j].ID
		case "nim":
			less = data[i].NIM < data[j].NIM
		case "name":
			less = strings.ToLower(data[i].Name) < strings.ToLower(data[j].Name)
		case "grade":
			less = data[i].Grade < data[j].Grade
		default:
			less = data[i].ID < data[j].ID
		}
		if desc {
			return !less
		}
		return less
	})
}

// paginate memotong slice student sesuai halaman & limit yang diminta,
// lalu mengembalikan potongannya beserta info PaginationMeta.
// Asumsi: `data` yang masuk ke sini SUDAH difilter & diurutkan,
// jadi total di sini adalah total SETELAH filter, bukan total keseluruhan tabel.
func paginate(data []Student, page, limit int) ([]Student, PaginationMeta) {
	if page < 1 {
		page = DefaultPage
	}
	if limit < 1 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	total := len(data)
	totalPages := (total + limit - 1) / limit // pembulatan ke atas
	if totalPages < 1 {
		totalPages = 1
	}

	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	meta := PaginationMeta{
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}

	return data[start:end], meta
}