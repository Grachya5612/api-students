package service
 
import (
    "strings"
    "unicode"
)
const minPasswordLength = 8
 
// Validasi Register dan Login sudah dipindahkan ke tag struct (validate:"...") 
// di app/model/auth.go. Fungsi ValidateRegister dan ValidateLogin dihapus.
// Gunakan helper.ValidateStruct(req) sebagai pengganti.
 
func checkPasswordStrength(password string) string {
    if len(password) < minPasswordLength {
        return "minimal 8 karakter"
    }
 
    var hasLetter, hasDigit bool
    for _, r := range password {
        switch {
        case unicode.IsLetter(r):
            hasLetter = true
        case unicode.IsDigit(r):
            hasDigit = true
        }
    }
 
    if !hasLetter || !hasDigit {
        return "harus memuat huruf dan angka"
    }
 
    // Daftar ini sengaja sangat pendek. Sistem sungguhan memakai daftar
    // berisi jutaan password yang pernah bocor.
    weak := map[string]bool{
        "password1": true, "12345678": true, "qwerty123": true,
        "admin123": true, "password123": true,
    }
    if weak[strings.ToLower(password)] {
        return "password terlalu umum"
	}	
	
	return ""
}
 
func isValidUsername(username string) bool {
    for _, r := range username {
        if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '.' && r != '_' {
            return false
        }
    }
    return true
}
