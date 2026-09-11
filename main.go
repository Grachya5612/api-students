package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"api-students/app/repository"
	"api-students/app/service"
	"api-students/config"
	"api-students/database"
	"api-students/helper"
)

const minSecretLength = 32

// main hanya berisi urutan perakitan.
// Tidak ada logika bisnis, query, atau handler di sini.
func main() {
	// 1. Konfigurasi dan logger
	config.LoadEnv()
	logger := config.NewLogger()

	// Rahasia diperiksa sebelum server menyala.
	jwtSecret := config.GetEnv("JWT_SECRET", "")
	if len(jwtSecret) < minSecretLength {
		logger.Error(
			"JWT_SECRET tidak diisi atau terlalu pendek",
			slog.Int("minimal_karakter", minSecretLength),
		)
		os.Exit(1)
	}

	// 2. Database
	pool, err := database.NewPool(context.Background())
	if err != nil {
		logger.Error(
			"gagal terhubung ke database",
			slog.String("error", err.Error()),
		)
		os.Exit(1)
	}
	defer pool.Close()

	// 3. JWT
	jwtManager := helper.NewJWTManager(
		jwtSecret,
		config.GetEnv("JWT_ISSUER", "praktikum-backend"),
		time.Duration(
			config.GetEnvInt("JWT_ACCESS_TTL_MINUTES", 15),
		)*time.Minute,
	)

	// 4. Repository
	userRepository := repository.NewUserRepository(pool)
	tokenRepository := repository.NewTokenRepository(pool)

	// 5. Service
	userService := service.NewUserService(userRepository)

	authService := service.NewAuthService(
		userRepository,
		tokenRepository,
		jwtManager,
		time.Duration(
			config.GetEnvInt("JWT_REFRESH_TTL_DAYS", 7),
		)*24*time.Hour,
	)

	// 6. Aplikasi
	app := config.NewApp(logger, pool, userService, jwtManager, authService)

	// 7. Server
	port := config.GetEnv("APP_PORT", "3000")

	go func() {
		if err := app.Listen(":" + port); err != nil {
			logger.Error(
				"server berhenti",
				slog.String("error", err.Error()),
			)
			os.Exit(1)
		}
	}()

	logger.Info(
		"server berjalan",
		slog.String("port", port),
	)

	// 8. Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	<-quit

	logger.Info("sinyal berhenti diterima, menutup server")

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	if err := app.ShutdownWithContext(ctx); err != nil {
		logger.Error(
			"gagal menutup server dengan rapi",
			slog.String("error", err.Error()),
		)
	}

	logger.Info("server berhenti dengan rapi")
}