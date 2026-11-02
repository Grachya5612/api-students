package model

import "time"

type Student struct {
	ID        int       `json:"id"`
	NIM       string    `json:"nim"`
	Name      string    `json:"name"`
	Grade     float64   `json:"grade"`
	IsActive  bool      `json:"is_active"`
	OwnerID   int       `json:"owner_id,omitempty"` // opsional, hanya diisi bila mahasiswa punya owner (user)
	CreatedAt time.Time `json:"created_at"`
}

// StudentCreateRequest dipakai untuk POST /students (semua field wajib)
// Aturan validasi ditulis sebagai tag pada struct.
// Custom validation: nimformat untuk format NIM yang sesuai domain
type StudentCreateRequest struct {
	NIM      string  `json:"nim" validate:"required,min=3,max=30,nimformat"`
	Name     string  `json:"name" validate:"required,min=3,max=100"`
	Grade    float64 `json:"grade" validate:"required,min=0,max=100"`
	IsActive bool    `json:"is_active"`
}

// StudentUpdateRequest dipakai untuk PUT /students/:id (replace penuh, semua wajib)
type StudentUpdateRequest struct {
	NIM      string  `json:"nim" validate:"required,min=3,max=30,nimformat"`
	Name     string  `json:"name" validate:"required,min=3,max=100"`
	Grade    float64 `json:"grade" validate:"required,min=0,max=100"`
	IsActive bool    `json:"is_active"`
}

// StudentPatchRequest dipakai untuk PATCH /students/:id (semua opsional, pakai pointer
// supaya bisa dibedakan "field tidak dikirim" vs "field dikirim nilai kosong").
// omitnil dipilih karena ia menyatakan maksud yang sebenarnya: lewati hanya bila nil.
// Custom validation: nimformat untuk format NIM yang sesuai domain
type StudentPatchRequest struct {
	NIM      *string  `json:"nim,omitempty" validate:"omitnil,min=3,max=30,nimformat"`
	Name     *string  `json:"name,omitempty" validate:"omitnil,min=3,max=100"`
	Grade    *float64 `json:"grade,omitempty" validate:"omitnil,min=0,max=100"`
	IsActive *bool    `json:"is_active,omitempty"`
}

// ListQuery menampung seluruh parameter query string pada GET /students,
// sudah dalam bentuk tervalidasi/ternormalisasi (bukan lagi string mentah).
type ListQuery struct {
	Page     int
	Limit    int
	Search   string
	Sort     string
	Order    string
	IsActive *bool
	GradeMin *float64
	GradeMax *float64
}

// Offset menghitung berapa baris yang dilewati untuk halaman ini.
// Dipakai langsung oleh repository sebagai nilai OFFSET pada SQL.
func (q ListQuery) Offset() int {
	return (q.Page - 1) * q.Limit
}

// Response adalah amplop seragam untuk seluruh endpoint, sukses maupun gagal.
type WebResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Meta    interface{} `json:"meta,omitempty"`
	Errors  interface{} `json:"errors,omitempty"`
}

// Meta berisi info paginasi pada response daftar mahasiswa.
type Meta struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// FieldError merepresentasikan satu kegagalan validasi pada satu field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ErrorResponse adalah bentuk response kegagalan yang terpusat,
// menggantikan WebResponse untuk error. Ia memiliki code dan request_id
// yang tidak ada pada WebResponse.
type ErrorResponse struct {
	Success   bool              `json:"success"`
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
	RequestID string            `json:"request_id,omitempty"`
}

// Cursor menandai posisi dalam keyset pagination.
// Menyertakan kedua field (created_at, id) untuk membandingkan secara unik.
type Cursor struct {
	CreatedAt time.Time
	ID        int
}

// CursorQuery menyimpan parameter query untuk keyset pagination.
type CursorQuery struct {
	After    *Cursor
	Limit    int
	Search   string
	IsActive *bool
}

// CursorMeta menggantikan Meta pada endpoint yang memakai cursor.
// Tidak ada Total dan TotalPages karena keduanya memerlukan COUNT(*).
type CursorMeta struct {
	Limit      int    `json:"limit"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}
