package controllers

import (
	"context"
<<<<<<< Updated upstream
=======
	"strings"
>>>>>>> Stashed changes
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"main-webbase/internal/models"
)

<<<<<<< Updated upstream
=======
// GetUserProfileHandler godoc
// @Summary      Get user profile by ID
// @Description  Returns profile information for a given user ID
// @Tags         Users
// @Produce      json
// @Param        id   path      string  true  "User ID"
// @Success      200  {object}  dto.UserProfileDTO
// @Failure      404  {object}  dto.ErrorResponse "user not found"
// @Failure      500  {object}  dto.ErrorResponse "internal server error"
// @Router       /users/profile/{id} [get]
func GetUserProfileHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Params("id")

		profile, err := services.GetUserProfile(c.Context(), userID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(profile)
	}
}

// GetUserProfileByQuery godoc
// @Summary      Get user profile by ID (query)
// @Description  Returns profile information for a given user ID via query param `id`
// @Tags         Users
// @Produce      json
// @Param        id   query     string  true  "User ID"
// @Success      200  {object}  dto.UserProfileDTO
// @Failure      400  {object}  dto.ErrorResponse "missing id"
// @Failure      500  {object}  dto.ErrorResponse "internal server error"
// @Router       /users/profile [get]
func GetUserProfileByQuery() fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Query("id")
		if userID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing id"})
		}
		profile, err := services.GetUserProfile(c.Context(), userID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(profile)
	}
}

// GetMyProfileHandler godoc
// @Summary      Get my profile
// @Description  Returns the profile of the currently authenticated user
// @Tags         Users
// @Produce      json
// @Success      200  {object}  dto.UserProfileDTO
// @Failure      401  {object}  dto.ErrorResponse "unauthorized"
// @Failure      500  {object}  dto.ErrorResponse "internal server error"
// @Router       /users/myprofile [get]
func GetMyProfileHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID, err := middleware.UIDFromLocals(c)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}

		profile, err := services.GetUserProfile(c.Context(), userID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(profile)
	}
}

>>>>>>> Stashed changes
// GetAllUser godoc
// @Summary Get all users
// @Description Returns all users in database
// @Tags users
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /users [get]
<<<<<<< Updated upstream
func GetAllUser(c *fiber.Ctx, client *mongo.Client) error {
	collection := client.Database("big_workspace").Collection("user")
=======
func GetAllUser() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Use the same collection name as auth ("users")
		collection := database.DB.Collection("users")
>>>>>>> Stashed changes

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// bson.D{} = get all documents
	cursor, err := collection.Find(ctx, bson.D{})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer cursor.Close(ctx)

<<<<<<< Updated upstream
	// .All = reads all documents and fill into users using cursor
	var users []models.User
	if err := cursor.All(ctx, &users); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Users fetched succesfully",
		"data":    users,
	})
=======
		// .All = reads all documents and fill into users using cursor
		var users []models.User
		if err := cursor.All(ctx, &users); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		include := strings.ToLower(strings.TrimSpace(c.Query("include")))
		includeMemberships := strings.Contains(include, "memberships")

		if !includeMemberships {
			return c.JSON(fiber.Map{
				"success": true,
				"message": "Users fetched succesfully",
				"data":    users,
			})
		}

		// Bulk fetch active memberships for these users
		ids := make([]bson.ObjectID, 0, len(users))
		idIndex := make(map[string]struct{}, len(users))
		for _, u := range users {
			if (u.ID != bson.ObjectID{}) {
				ids = append(ids, u.ID)
				idIndex[u.ID.Hex()] = struct{}{}
			}
		}
		memCol := database.DB.Collection("memberships")
		memCur, err := memCol.Find(ctx, bson.M{"user_id": bson.M{"$in": ids}, "active": true})
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer memCur.Close(ctx)

		type MemRow struct {
			ID          bson.ObjectID `bson:"_id"`
			UserID      bson.ObjectID `bson:"user_id"`
			OrgPath     string        `bson:"org_path"`
			PositionKey string        `bson:"position_key"`
			Active      bool          `bson:"active"`
		}
		memByUser := map[string][]fiber.Map{}
		for memCur.Next(ctx) {
			var m MemRow
			if err := memCur.Decode(&m); err == nil {
				uid := m.UserID.Hex()
				if _, ok := idIndex[uid]; !ok {
					continue
				}
				memByUser[uid] = append(memByUser[uid], fiber.Map{
					"_id":          m.ID.Hex(),
					"org_path":     m.OrgPath,
					"position_key": m.PositionKey,
					"active":       m.Active,
				})
			}
		}

		// Build output with memberships attached
		out := make([]fiber.Map, 0, len(users))
		for _, u := range users {
			out = append(out, fiber.Map{
				"_id":         u.ID.Hex(),
				"firstname":   u.FirstName,
				"lastname":    u.LastName,
				"thaiprefix":  u.ThaiPrefix,
				"gender":      u.Gender,
				"type_person": u.TypePerson,
				"student_id":  u.StudentID,
				"advisor_id":  u.AdvisorID,
				"email":       u.Email,
				"memberships": memByUser[u.ID.Hex()],
			})
		}

		return c.JSON(fiber.Map{
			"success": true,
			"message": "Users fetched succesfully",
			"data":    out,
		})
	}
>>>>>>> Stashed changes
}

// GetUserBy godoc
// @Summary Get user by field
// @Description Get a user by ID, firstname, lastname, etc.
// @Tags users
// @Produce json
// @Param value path string true "Search value"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /user/{field}/{value} [get]
<<<<<<< Updated upstream
func GetUserBy(c *fiber.Ctx, client *mongo.Client, field string) error {
	collection := client.Database("big_workspace").Collection("user")
	value := c.Params("value")
=======
func GetUserBy(field string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Use the same collection name as auth ("users")
		collection := database.DB.Collection("users")
		value := c.Params("value")
>>>>>>> Stashed changes

	var filter bson.M
	if field == "_id" {
		objID, err := bson.ObjectIDFromHex(value)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
		}
		filter = bson.M{"_id": objID}
	} else {
		filter = bson.M{field: value}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	var users []models.User
	if err := cursor.All(ctx, &users); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	if len(users) == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "No users found"})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"data":    users,
	})
}

// DeleteUser godoc
// @Summary Delete user by ID
// @Description Delete a user with given ID
// @Tags users
// @Param id path string true "User ID"
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /users/{id} [delete]
<<<<<<< Updated upstream
func DeleteUser(c *fiber.Ctx, client *mongo.Client) error {
	collection := client.Database("big_workspace").Collection("user")
	id := c.Params("id")

	objID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
=======
func DeleteUser() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Only admin/root can delete users
		if !isRootByPath(viewerFrom(c)) {
			return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
		}

		id := c.Params("id")
		if _, err := bson.ObjectIDFromHex(id); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid ID"})
		}

		if err := services.DeleteUserCascade(c.Context(), id); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(fiber.Map{
			"success": true,
			"message": "User deleted successfully",
		})
>>>>>>> Stashed changes
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := collection.DeleteOne(ctx, bson.M{"_id": objID})
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	if res.DeletedCount == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "User not found"})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "User deleted successfully",
	})
}
