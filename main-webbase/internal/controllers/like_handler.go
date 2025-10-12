package controllers

import (
	"context"
	"main-webbase/dto"
	"main-webbase/internal/services"
	mid "main-webbase/internal/middleware"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func LikeUnlikeHandler(client *mongo.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		user_id, err := mid.UIDObjectID(c)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).
				JSON(dto.ErrorResponse{Error: "missing userId in context"})
		}

		var body dto.LikeRequestDTO
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{Error: "invalid body"})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		status, payload := services.Like(ctx, client, body, user_id)
		return c.Status(status).JSON(payload)
	}
}