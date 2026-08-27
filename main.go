package main

import (
	"context"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"

	"api-students/app/repository"
	"api-students/config"
	"api-students/database"
)

func main() {
	// 1. Konfigurasi
	config.LoadEnv()

	// 2. Koneksi basis data (connection pool)
	pool, err := database.NewPool(context.Background())
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	// 3. Perakitan: pool -> repository -> handler
	studentRepository := repository.NewStudentRepository(pool)
	studentHandler := NewStudentHandler(studentRepository)

	// 4. Aplikasi
	app := fiber.New()

	api := app.Group("/api/v1")

	// Endpoint kesehatan sekarang ikut memeriksa basis data, bukan cuma
	// memastikan server Fiber-nya hidup.
	api.Get("/health", func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.UserContext(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			return fail(c, fiber.StatusServiceUnavailable, "database tidak dapat dihubungi")
		}
		return ok(c, "server dan database berjalan", nil)
	})

	s := api.Group("/students", requireJSON)
	s.Get("/", studentHandler.List)
	s.Get("/:id", studentHandler.Get)
	s.Post("/", studentHandler.Create)
	s.Put("/:id", studentHandler.Replace)
	s.Patch("/:id", studentHandler.Patch)
	s.Delete("/:id", studentHandler.Delete)

	port := config.GetEnv("APP_PORT", "3000")
	log.Fatal(app.Listen(":" + port))
}
