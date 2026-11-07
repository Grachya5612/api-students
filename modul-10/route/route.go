package route

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"api-students/app/service"
	"api-students/helper"
	"api-students/middleware"
)

type Dependencies struct {
	Pool                *pgxpool.Pool
	JWT                 *helper.JWTManager
	Permissions         *helper.PermissionSet
	UserService         *service.UserService
	StudentService      *service.StudentService
	AuthService         *service.AuthService
	CourseService       *service.CourseService
	ReviewService       *service.ReviewService
	SupervisionService  *service.SupervisionService
	MongoClient         *mongo.Client
	PortfolioService    *service.PortfolioService
	NotificationService *service.NotificationService
}

func Register(app *fiber.App, deps Dependencies) {
	api := app.Group("/api/v1")

	// --- publik ---
	api.Get("/health", healthCheck(deps.Pool, deps.MongoClient))

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

	perms := deps.Permissions

	// --- pengguna ---
	users := api.Group("/users",
		middleware.RequireJSON,
		middleware.RequireAuth(deps.JWT),
	)

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

	// Hak akses berdasarkan kepemilikan diperiksa di service.
	users.Get("/:id", deps.UserService.Get)
	users.Put("/:id", deps.UserService.Replace)
	users.Patch("/:id", deps.UserService.Patch)

	// --- mahasiswa ---
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

	students.Get("/:id", deps.StudentService.Get)
	students.Put("/:id", deps.StudentService.Replace)
	students.Patch("/:id", deps.StudentService.Patch)

	// --- kelas dan KRS ---
	courses := api.Group("/courses",
		middleware.RequireAuth(deps.JWT),
	)

	// "/mine" didaftarkan sebelum "/:id".
	courses.Get("/", deps.CourseService.List)
	courses.Get("/mine", deps.CourseService.Mine)
	courses.Get("/:id", deps.CourseService.Get)
	courses.Post("/:id/enroll", deps.CourseService.Enroll)
	courses.Delete("/:id/enroll", deps.CourseService.Drop)
	courses.Patch("/:id",
		middleware.RequireJSON,
		middleware.RequirePermission(perms, "course:manage"),
		deps.CourseService.UpdateQuota,
	)
	courses.Get("/:id/reviews", deps.ReviewService.ListByCourse)
	courses.Get("/:id/reviews/summary", deps.ReviewService.Summary)
	courses.Post("/:id/reviews",
		middleware.RequireJSON,
		deps.ReviewService.Create,
	)

	// --- ulasan kelas (MongoDB) ---
	reviews := api.Group("/reviews",
		middleware.RequireAuth(deps.JWT),
	)
	reviews.Get("/:id", deps.ReviewService.Get)
	reviews.Patch("/:id", middleware.RequireJSON, deps.ReviewService.Update)
	reviews.Delete("/:id", deps.ReviewService.Delete)
	reviews.Post("/:id/replies", middleware.RequireJSON, deps.ReviewService.Reply)
	reviews.Post("/:id/helpful", deps.ReviewService.MarkHelpful)

	// --- portofolio mahasiswa (MongoDB) ---
	// Didaftarkan langsung di api, bukan di group students, supaya tidak
	// terkena RequireJSON di level group.
	ph := newPortfolioHandler(deps.PortfolioService)

	api.Get("/students/:id/portfolio",
		middleware.RequireAuth(deps.JWT),
		ph.get,
	)
	api.Put("/students/:id/portfolio",
		middleware.RequireAuth(deps.JWT),
		middleware.RequireJSON,
		ph.upsert,
	)
	api.Post("/students/:id/portfolio/projects",
		middleware.RequireAuth(deps.JWT),
		middleware.RequireJSON,
		ph.addProject,
	)
	api.Delete("/students/:id/portfolio/projects/:projectId",
		middleware.RequireAuth(deps.JWT),
		ph.deleteProject,
	)
	api.Get("/portfolios/stats/technologies",
		middleware.RequireAuth(deps.JWT),
		ph.techStats,
	)

	// --- pembimbing tugas akhir ---
	supervisors := api.Group("/supervisors",
		middleware.RequireJSON,
		middleware.RequireAuth(deps.JWT),
	)
	supervisors.Post("/:id/assign",
		middleware.RequirePermission(perms, "supervision:manage"),
		deps.SupervisionService.Assign,
	)
	supervisors.Patch("/:id",
		middleware.RequirePermission(perms, "supervision:manage"),
		deps.SupervisionService.UpdateQuota,
	)

	supervisions := api.Group("/supervisions",
		middleware.RequireJSON,
		middleware.RequireAuth(deps.JWT),
	)
	supervisions.Post("/switch",
		middleware.RequirePermission(perms, "supervision:manage"),
		deps.SupervisionService.Switch,
	)

	// --- pertukaran kelas ---
	enrollments := api.Group("/enrollments",
		middleware.RequireJSON,
		middleware.RequireAuth(deps.JWT),
	)
	enrollments.Post("/swap", deps.CourseService.Swap)

	// --- notifikasi (diisi oleh worker, dibaca lewat API) ---
	notifications := api.Group("/notifications",
		middleware.RequireJSON,
		middleware.RequireAuth(deps.JWT),
	)
	notifications.Get("/mine", deps.NotificationService.Mine)

	admin := api.Group("/admin",
		middleware.RequireJSON,
		middleware.RequireAuth(deps.JWT),
	)
	admin.Get("/queue",
		middleware.RequirePermission(perms, "queue:monitor"),
		deps.NotificationService.Monitor,
	)
}

// healthCheck memeriksa DUA database. Aplikasi dianggap sehat hanya
// bila keduanya dapat dihubungi, dan pesan kegagalannya menyebut yang mana.
func healthCheck(pool *pgxpool.Pool, mongoClient *mongo.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.UserContext(), 2*time.Second)
		defer cancel()

		if err := pool.Ping(ctx); err != nil {
			return helper.ServiceUnavailable("PostgreSQL tidak dapat dihubungi")
		}

		if err := mongoClient.Ping(ctx, nil); err != nil {
			return helper.ServiceUnavailable("MongoDB tidak dapat dihubungi")
		}

		return helper.Success(c, fiber.StatusOK, "server, PostgreSQL, dan MongoDB berjalan", nil)
	}
}