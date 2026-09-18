package route

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"api-students/app/service"
	"api-students/helper"
	"api-students/middleware"
)

type Dependencies struct {
	Pool           *pgxpool.Pool
	JWT            *helper.JWTManager
	Permissions    *helper.PermissionSet
	UserService    *service.UserService
	StudentService *service.StudentService
	AuthService    *service.AuthService
}

func Register(app *fiber.App, deps Dependencies) {
	api := app.Group("/api/v1")

	// --- publik ---
	api.Get("/health", healthCheck(deps.Pool))

	// --- autentikasi ---
	auth := api.Group("/auth", middleware.RequireJSON)
	auth.Post("/register", deps.AuthService.Register)
	auth.Post("/login", middleware.LoginRateLimiter(), deps.AuthService.Login)
	auth.Post("/refresh", deps.AuthService.Refresh)
	auth.Post("/logout", deps.AuthService.Logout)
	auth.Get("/me",
		middleware.RequireAuth(deps.JWT),
		deps.AuthService.Me,
	)

	// --- wajib login, hak akses diperiksa per endpoint ---
	users := api.Group("/users",
		middleware.RequireJSON,
		middleware.RequireAuth(deps.JWT),
	)

	perms := deps.Permissions

	// Hak dapat diputuskan tanpa melihat data -> middleware.
	users.Get("/",
		middleware.RequirePermission(perms, "user:list"),
		deps.UserService.List,
	)

	users.Post("/",
		middleware.RequirePermission(perms, "user:update:any"),
		deps.UserService.Create,
	)

	users.Delete("/:id",
		middleware.RequirePermission(perms, "user:delete"),
		deps.UserService.Delete,
	)

	users.Patch("/:id/role",
		middleware.RequirePermission(perms, "role:assign"),
		deps.UserService.AssignRole,
	)

	students := api.Group("/students",
		middleware.RequireJSON,
		middleware.RequireAuth(deps.JWT),
	)

	students.Get("/",
		middleware.RequirePermission(perms, "student:list"),
		deps.StudentService.List,
	)

	students.Post("/",
		middleware.RequirePermission(perms, "student:create"),
		deps.StudentService.Create,
	)

	students.Delete("/:id",
		middleware.RequirePermission(perms, "student:delete"),
		deps.StudentService.Delete,
	)

	students.Get("/:id",
		deps.StudentService.Get,
	)

	students.Put("/:id",
		deps.StudentService.Replace,
	)

	students.Patch("/:id",
		deps.StudentService.Patch,
	)

	// Hak bergantung pada kepemilikan data -> diperiksa di service.
	users.Get("/:id", deps.UserService.Get)
	users.Put("/:id", deps.UserService.Replace)
	users.Patch("/:id", deps.UserService.Patch)
}

func healthCheck(pool *pgxpool.Pool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(
			c.Context(),
			5*time.Second,
		)
		defer cancel()

		if err := pool.Ping(ctx); err != nil {
			return helper.Fail(
				c,
				fiber.StatusServiceUnavailable,
				"database tidak terhubung",
			)
		}

		return helper.Success(
			c,
			fiber.StatusOK,
			"server dan database berjalan",
			nil,
		)
	}
}
