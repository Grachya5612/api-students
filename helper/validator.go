package helper

import (
	"errors"
	"reflect"
	"strings"
	"unicode"

	"github.com/go-playground/validator/v10"
)

// validate dibuat SEKALI untuk seluruh aplikasi.
//
// validator.New() melakukan refleksi dan menyimpan hasilnya dalam cache
// internal. Membuatnya ulang pada setiap request berarti membuang cache
// tersebut berkali-kali — mahal dan tidak ada gunanya.
var validate = newValidator()

func newValidator() *validator.Validate {
	v := validator.New()

	// Tanpa ini, pesan error menyebut nama field Go (\"Username\"),
	// padahal client mengirim dan membaca nama JSON (\"username\").
	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		name := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]
		if name == "" || name == "-" {
			return field.Name
		}
		return name
	})

	// Aturan buatan sendiri. Aturan yang tidak disediakan library tetap
	// ditulis secara deklaratif sebagai tag, bukan dikembalikan menjadi
	// pemeriksaan manual yang tersebar di dalam service.
	_ = v.RegisterValidation("nospace", func(fl validator.FieldLevel) bool {
		return !strings.ContainsAny(fl.Field().String(), " \t\n\r")
	})

	_ = v.RegisterValidation("username", func(fl validator.FieldLevel) bool {
		for _, r := range fl.Field().String() {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) &&
				r != '.' && r != '_' {
				return false
			}
		}
		return true
	})

	_ = v.RegisterValidation("strongpassword", func(fl validator.FieldLevel) bool {
		return passwordStrength(fl.Field().String()) == ""
	})

	// Custom validator untuk NIM: minimal 3 karakter, hanya alphanumeric + dash
	// Format contoh: "24051234" atau "NIM-24051234"
	_ = v.RegisterValidation("nimformat", func(fl validator.FieldLevel) bool {
		nim := fl.Field().String()
		if len(nim) < 3 {
			return false
		}
		for _, r := range nim {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' {
				return false
			}
		}
		return true
	})

	return v
}

// ValidateStruct menjalankan seluruh aturan pada tag struct dan
// mengembalikan peta nama field ke pesan berbahasa Indonesia.
// Mengembalikan nil berarti tidak ada pelanggaran.
func ValidateStruct(s any) map[string]string {
	err := validate.Struct(s)
	if err == nil {
		return nil
	}

	// Terjadi bila yang dikirim bukan struct — itu kesalahan programmer,
	// bukan kesalahan pemakai API. Jangan diam-diam dianggap valid.
	var invalid *validator.InvalidValidationError
	if errors.As(err, &invalid) {
		return map[string]string{"_": "objek yang divalidasi tidak sah"}
	}

	var fieldErrors validator.ValidationErrors
	if !errors.As(err, &fieldErrors) {
		return map[string]string{"_": "validasi gagal"}
	}

	result := make(map[string]string, len(fieldErrors))
	for _, fe := range fieldErrors {
		if _, exists := result[fe.Field()]; !exists {
			result[fe.Field()] = messageFor(fe)
		}
	}
	return result
}

// messageFor menerjemahkan nama tag menjadi kalimat yang dapat dibaca
// pemakai. Daftar ini terpusat: menambah satu tag baru cukup menambah
// satu case di sini, tidak menyebar ke banyak file.
func messageFor(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "wajib diisi"
	case "email":
		return "format email tidak valid"
	case "min":
		if fe.Kind() == reflect.String {
			return "minimal " + fe.Param() + " karakter"
		}
		return "nilai minimal " + fe.Param()
	case "max":
		if fe.Kind() == reflect.String {
			return "maksimal " + fe.Param() + " karakter"
		}
		return "nilai maksimal " + fe.Param()
	case "alphanum":
		return "hanya boleh berisi huruf dan angka"
	case "nospace":
		return "tidak boleh mengandung spasi"
	case "username":
		return "hanya boleh huruf, angka, titik, dan garis bawah"
	case "strongpassword":
		// Type assertion memakai bentuk DUA nilai, bukan satu. Bentuk
		// satu nilai akan panic bila suatu saat tag ini terpasang pada
		// field bukan string — mematikan server hanya karena salah tag.
		if value, ok := fe.Value().(string); ok {
			return passwordStrength(value)
		}
		return "password tidak memenuhi syarat"
	case "oneof":
		return "harus salah satu dari: " +
			strings.ReplaceAll(fe.Param(), " ", ", ")
	case "nimformat":
		return "format NIM tidak valid (contoh: 24051234 atau NIM-24051234)"
	case "omitnil":
		// omitnil tidak seharusnya muncul di sini karena ia hanya
		// memerintahkan validator untuk lewati field, bukan melakukan
		// validasi sendiri. Tapi jaring pengaman tidak ada salahnya.
		return "tidak memenuhi aturan"
	default:
		// Jaring pengaman. Bila muncul di log, artinya ada tag yang
		// dipakai tetapi belum diterjemahkan di sini.
		return "tidak memenuhi aturan " + fe.Tag()
	}
}

// passwordStrength memeriksa kualitas password dan mengembalikan
// pesan kesalahan (atau string kosong bila kuat).
// Dipindahkan apa adanya dari auth_rules.go.
func passwordStrength(password string) string {
	// Referensi umum: SplashData Top 100 Passwords
	commonPasswords := map[string]bool{
		"password":     true,
		"123456":       true,
		"12345678":     true,
		"qwerty":       true,
		"abc123":       true,
		"monkey":       true,
		"1234567":      true,
		"letmein":      true,
		"trustno1":     true,
		"dragon":       true,
		"baseball":     true,
		"111111":       true,
		"iloveyou":     true,
		"master":       true,
		"sunshine":     true,
		"ashley":       true,
		"bailey":       true,
		"passw0rd":     true,
		"shadow":       true,
		"123123":       true,
		"654321":       true,
		"superman":     true,
		"qazwsx":       true,
		"michael":      true,
		"football":     true,
		"password123":  true,
		"tanpaangka":   true,
		"senangsenang": true,
	}

	if len(password) < 8 {
		return "minimal 8 karakter"
	}

	if commonPasswords[strings.ToLower(password)] {
		return "password terlalu umum"
	}

	hasLetter := false
	hasDigit := false

	for _, r := range password {
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if unicode.IsDigit(r) {
			hasDigit = true
		}
	}

	if !hasLetter || !hasDigit {
		return "harus memuat huruf dan angka"
	}

	return ""
}
