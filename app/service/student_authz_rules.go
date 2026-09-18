package service

import (
	"api-students/app/model"
	"api-students/helper"
)

// CanAccessStudent menentukan apakah current user boleh mengakses
// data student tertentu.
//
// Owner selalu boleh mengakses datanya sendiri.
// Jika bukan owner, user harus memiliki permission anyPermission.
//
// Fungsi ini sengaja dibuat pure: tidak bergantung pada Fiber,
// repository, database, atau HTTP.
func CanAccessStudent(
	current model.AuthUser,
	ownerID int,
	perms *helper.PermissionSet,
	anyPermission string,
) bool {
	if current.UserID == ownerID {
		return true
	}

	return perms.Can(current.Role, anyPermission)
}