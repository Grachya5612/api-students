package service

import (
	"testing"

	"api-students/app/model"
)

// TestCountTotalPages menguji perhitungan jumlah halaman.
func TestCountTotalPages(t *testing.T) {
	cases := []struct {
		total, limit, want int
	}{
		{0, 10, 0},
		{1, 10, 1},
		{10, 10, 1},
		{11, 10, 2},
		{137, 20, 7},
	}

	for _, tc := range cases {
		if got := CountTotalPages(tc.total, tc.limit); got != tc.want {
			t.Errorf(
				"total=%d limit=%d: harap %d, dapat %d",
				tc.total,
				tc.limit,
				tc.want,
				got,
			)
		}
	}
}

// TestValidateCreate menguji validasi POST/create user.
func TestValidateCreate(t *testing.T) {
	req := model.CreateUserRequest{
		Username: "sari",
		Email:    "sari@mail.com",
		Password: "password123",
	}

	errs := ValidateCreate(req)

	if len(errs) != 0 {
		t.Fatalf("data valid seharusnya tidak menghasilkan error: %v", errs)
	}

	// Uji data tidak valid.
	invalidReq := model.CreateUserRequest{
		Username: "",
		Email:    "sari",
		Password: "123",
	}

	errs = ValidateCreate(invalidReq)

	if len(errs) != 3 {
		t.Fatalf("seharusnya ada 3 error validasi, dapat %d: %v", len(errs), errs)
	}

	if _, ok := errs["username"]; !ok {
		t.Error("username seharusnya menghasilkan error")
	}

	if _, ok := errs["email"]; !ok {
		t.Error("email seharusnya menghasilkan error")
	}

	if _, ok := errs["password"]; !ok {
		t.Error("password seharusnya menghasilkan error")
	}
}

// TestValidateReplace menguji validasi PUT/replace user.
func TestValidateReplace(t *testing.T) {
	req := model.ReplaceUserRequest{
		Username: "sari",
		Email:    "sari@mail.com",
		IsActive: true,
	}

	errs := ValidateReplace(req)

	if len(errs) != 0 {
		t.Fatalf("data valid seharusnya tidak menghasilkan error: %v", errs)
	}

	// Uji data tidak valid.
	invalidReq := model.ReplaceUserRequest{
		Username: "",
		Email:    "sari",
		IsActive: true,
	}

	errs = ValidateReplace(invalidReq)

	if len(errs) != 2 {
		t.Fatalf("seharusnya ada 2 error validasi, dapat %d: %v", len(errs), errs)
	}

	if _, ok := errs["username"]; !ok {
		t.Error("username seharusnya menghasilkan error")
	}

	if _, ok := errs["email"]; !ok {
		t.Error("email seharusnya menghasilkan error")
	}
}

// TestApplyPatch menguji penerapan perubahan PATCH.
func TestApplyPatch(t *testing.T) {
	initial := model.User{
		ID:       1,
		Username: "sari",
		Email:    "sari@mail.com",
		IsActive: true,
	}

	inactive := false

	result, errs := ApplyPatch(
		initial,
		model.PatchUserRequest{
			IsActive: &inactive,
		},
	)

	if len(errs) != 0 {
		t.Fatalf("tidak seharusnya ada error: %v", errs)
	}

	if result.IsActive {
		t.Error("is_active seharusnya berubah menjadi false")
	}

	if result.Username != "sari" {
		t.Error("field yang tidak dikirim seharusnya tidak berubah")
	}

	if result.Email != "sari@mail.com" {
		t.Error("field yang tidak dikirim seharusnya tidak berubah")
	}
}