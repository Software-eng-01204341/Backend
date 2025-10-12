// @title Fiber MongoDB API
// @version 1.0
// @description This is a sample server for user management.
// @host localhost:8000
// @BasePath /

package main

import (
    "log"
    "os"
    "strings"

    _ "main-webbase/docs"

    "github.com/gofiber/fiber/v2"
    "github.com/gofiber/fiber/v2/middleware/cors"
    "github.com/gofiber/swagger"
    "github.com/joho/godotenv"

    "main-webbase/config"
    "main-webbase/database"
    "main-webbase/internal/middleware"
    "main-webbase/internal/routes"
)

func main() {

	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ Warning: .env file not found, using system environment variables")
	}

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		panic("JWT_SECRET is required")
	}
	log.Printf("env: JWT_SECRET len=%d", len(os.Getenv("JWT_SECRET")))

	// Load configuration
	cfg := config.LoadConfig()

	// Connect to the database
	client := database.ConnectMongo(cfg.MongoURI, cfg.MongoDB)
	defer client.Disconnect(nil)

	db := client.Database("unicom")

    // Fiber app
    app := fiber.New()

    // CORS: allow dev frontend and handle preflight before auth
    allowed := os.Getenv("FRONTEND_ORIGINS")
    if allowed == "" {
        allowed = "http://localhost:5173,http://127.0.0.1:5173"
    }
    app.Use(cors.New(cors.Config{
        AllowOrigins:     strings.Join(strings.Split(allowed, ","), ","),
        AllowMethods:     "GET,POST,PUT,DELETE,PATCH,OPTIONS",
        AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
        ExposeHeaders:    "Authorization",
        AllowCredentials: true,
    }))

    // Short-circuit OPTIONS early (in case any middleware below would block it)
    app.Use(func(c *fiber.Ctx) error {
        if c.Method() == fiber.MethodOptions {
            return c.SendStatus(fiber.StatusNoContent)
        }
        return c.Next()
    })

	// app.Use(func(c *fiber.Ctx) error {
	// 	c.Locals("user_id", "68bd6ff6f80438824239b8a9")
	// 	c.Locals("is_Root", false)
	// 	return c.Next()
	// })

	// Swagger API document for Faisu and Vincy
	app.Get("/docs/*", swagger.HandlerDefault)

	// Health
	app.Get("/healthz", func(c *fiber.Ctx) error { return c.SendString("ok") })

    // Get JWT with login (open)
    routes.SetupAuth(app)

    // Authn/Z for protected routes
    app.Use(middleware.JWTUidOnly(secret))
    app.Use(middleware.InjectViewer(db))

	// Routes
	routes.SetupRoutesUser(app)
	// routes.SetupRoutesAbility(app)
	routes.SetupRoutesOrg(app)
	routes.SetupRoutesMembership(app)
	routes.SetupRoutesPosition(app)
	routes.SetupRoutesPolicy(app)
	// routes.SetupRoutesPost(app, client)
	routes.SetupRoutesEvent(app, client)
	routes.SetupRoutesPost(app, client)
	routes.CommentRoutes(app, client)
	routes.LikeRoutes(app, client)

	// RUN SERVER
	log.Fatal(app.Listen(":" + cfg.Port))
}
