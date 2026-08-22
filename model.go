package main

// Student merepresentasikan entitas mahasiswa.
// NIM ditambahkan sebagai penanda unik selain ID (auto-increment internal).
type Student struct {
	ID       int     `json:"id"`
	NIM      string  `json:"nim"`
	Name     string  `json:"name"`
	Grade    float64 `json:"grade"`
	IsActive bool    `json:"is_active"`
}

// ===================== REQUEST STRUCTS =====================
// Dipisah per method karena kebutuhan validasinya beda:
// - POST  : semua field wajib diisi (data baru)
// - PUT   : semua field wajib diisi (replace total)
// - PATCH : semua field opsional, pakai pointer supaya bisa
//           membedakan "field tidak dikirim" vs "field dikirim nilai kosong"

// StudentCreateRequest dipakai untuk POST /students
type StudentCreateRequest struct {
	NIM      string  `json:"nim" validate:"required"`
	Name     string  `json:"name" validate:"required"`
	Grade    float64 `json:"grade"`
	IsActive bool    `json:"is_active"`
}

// StudentUpdateRequest dipakai untuk PUT /students/:id (replace penuh)
type StudentUpdateRequest struct {
	NIM      string  `json:"nim" validate:"required"`
	Name     string  `json:"name" validate:"required"`
	Grade    float64 `json:"grade"`
	IsActive bool    `json:"is_active"`
}

// StudentPatchRequest dipakai untuk PATCH /students/:id (update sebagian)
// Semua field pointer & "omitempty" supaya field yang tidak dikirim
// client tetap bernilai nil dan tidak menimpa data yang sudah ada.
type StudentPatchRequest struct {
	NIM      *string  `json:"nim,omitempty"`
	Name     *string  `json:"name,omitempty"`
	Grade    *float64 `json:"grade,omitempty"`
	IsActive *bool    `json:"is_active,omitempty"`
}

// ===================== RESPONSE ENVELOPE =====================
// Response adalah "amplop" seragam untuk SELURUH endpoint,
// baik yang sukses maupun yang gagal, supaya konsumen API
// (frontend/klien) selalu tahu bentuk response yang konsisten.
// Meta dipakai khusus untuk endpoint yang butuh info tambahan,
// misalnya info paginasi pada GET /api/v1/students.
type Response struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Meta    interface{} `json:"meta,omitempty"`
}

// PaginationMeta berisi info paginasi yang dikembalikan di field "meta"
// pada response daftar mahasiswa. Nama field JSON ("total") sengaja
// disamakan persis dengan yang diminta di ketentuan tugas.
type PaginationMeta struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// FieldError merepresentasikan satu kegagalan validasi pada satu field.
// Dipakai di response 422, supaya client tahu PERSIS field mana yang
// salah dan kenapa -- bukan cuma pesan generik "validasi gagal".
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}