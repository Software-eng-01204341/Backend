package middleware

import (
	"context"
	"time"

	"main-webbase/database"
	"main-webbase/internal/accessctx"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func InjectViewer(db *mongo.Database) fiber.Handler {
	return func(c *fiber.Ctx) error {
		uidHex, ok := c.Locals("user_id").(string)
		if !ok || uidHex == "" {
			return fiber.ErrUnauthorized
		}

		uid, err := bson.ObjectIDFromHex(uidHex) // v2 ใช้ bson.ObjectIDFromHex
		if err != nil {
			return fiber.ErrUnauthorized
		}

		ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
		defer cancel()

		v, err := accessctx.BuildViewerAccess(ctx, database.DB, uid)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return fiber.ErrUnauthorized
			}
			return err
		}
		c.Locals("viewer", v)
		return c.Next()
	}
}
