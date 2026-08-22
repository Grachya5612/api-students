package main

import (
	"log"

	"github.com/gofiber/fiber/v2"
)

func main() {
	app := fiber.New()

	// Routing untuk resource students, di-group dengan prefix /api/v1
	v1 := app.Group("/api/v1")
	v1.Get("/students", GetAllStudents)
	v1.Get("/students/:id", GetStudentByID)
	v1.Post("/students", CreateStudent)
	v1.Put("/students/:id", UpdateStudent)
	v1.Patch("/students/:id", PatchStudent)
	v1.Delete("/students/:id", DeleteStudent)

	log.Fatal(app.Listen(":3000"))
}
