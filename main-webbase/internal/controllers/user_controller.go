package controllers

import (
    "context"
    "time"
    "strings"
    "fmt"
	"path/filepath"
	"golang.org/x/crypto/bcrypt"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/bson"

	"main-webbase/database"
	"main-webbase/internal/middleware"
	"main-webbase/internal/models"
	"main-webbase/internal/services"
)

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

func UpdateMyProfileHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get user ID from middleware
		userID, err := middleware.UIDFromLocals(c)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}

		update := bson.M{}

		// Handle profile picture upload
		file, err := c.FormFile("file")
		if err == nil && file != nil {
			timestamp := time.Now().UnixNano() / 1e6
			ext := filepath.Ext(file.Filename)
			filename := fmt.Sprintf("%s_%d%s", userID, timestamp, ext)
			savePath := filepath.Join("/var/www/html/uploads", filename)

			if err := c.SaveFile(file, savePath); err != nil {
				return c.Status(fiber.StatusInternalServerError).
					JSON(fiber.Map{"error": "failed to save file"})
			}

			// Update MongoDB with public URL
			publicURL := fmt.Sprintf("http://45.144.166.252:46602/uploads/%s", filename)
			update["profile_pic"] = publicURL
		}

		// Handle other profile fields from form or JSON
		req := struct {
			FirstName   *string `json:"FirstName"`
			LastName    *string `json:"LastName"`
			ThaiPrefix  *string `json:"ThaiPrefix"`
			Gender      *string `json:"Gender"`
			TypePerson  *string `json:"TypePerson"`
			StudentID   *string `json:"StudentID"`
			AdvisorID   *string `json:"AdvisorID"`
			Password    *string `json:"Password"`
		}{}

		// Attempt JSON parsing first
		_ = c.BodyParser(&req)
		// Fallback to form values if BodyParser didn't populate fields
		if req.FirstName == nil {
			if val := c.FormValue("FirstName"); val != "" {
				req.FirstName = &val
			}
		}
		if req.LastName == nil {
			if val := c.FormValue("LastName"); val != "" {
				req.LastName = &val
			}
		}
		if req.ThaiPrefix == nil {
			if val := c.FormValue("ThaiPrefix"); val != "" {
				req.ThaiPrefix = &val
			}
		}
		if req.Gender == nil {
			if val := c.FormValue("Gender"); val != "" {
				req.Gender = &val
			}
		}
		if req.TypePerson == nil {
			if val := c.FormValue("TypePerson"); val != "" {
				req.TypePerson = &val
			}
		}
		if req.StudentID == nil {
			if val := c.FormValue("StudentID"); val != "" {
				req.StudentID = &val
			}
		}
		if req.AdvisorID == nil {
			if val := c.FormValue("AdvisorID"); val != "" {
				req.AdvisorID = &val
			}
		}
		if req.Password == nil {
			if val := c.FormValue("Password"); val != "" {
				req.Password = &val
			}
		}

		// Prepare update document
		if req.FirstName != nil {
			update["firstname"] = *req.FirstName
		}
		if req.LastName != nil {
			update["lastname"] = *req.LastName
		}
		if req.ThaiPrefix != nil {
			update["thaiprefix"] = *req.ThaiPrefix
		}
		if req.Gender != nil {
			update["gender"] = *req.Gender
		}
		if req.TypePerson != nil {
			update["type_person"] = *req.TypePerson
		}
		if req.StudentID != nil {
			update["student_id"] = *req.StudentID
		}
		if req.AdvisorID != nil {
			update["advisor_id"] = *req.AdvisorID
		}
        if req.Password != nil {
            hashedPassword, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
            if err != nil {
                return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to hash password"})
            }
            update["password_hash"] = string(hashedPassword)
        }


		update["updated_at"] = time.Now()

		if len(update) == 1 && update["updated_at"] != nil {
			// Only updated_at is set -> nothing else to update
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no fields to update"})
		}

		collection := database.DB.Collection("users")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		objID, err := bson.ObjectIDFromHex(userID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user ID"})
		}

		res, err := collection.UpdateOne(ctx, bson.M{"_id": objID}, bson.M{"$set": update})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		if res.MatchedCount == 0 {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
		}

		// Return updated profile
		profile, err := services.GetUserProfile(c.Context(), userID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(profile)
	}
}

// GetAllUser godoc
// @Summary Get all users
// @Description Returns all users in database
// @Tags users
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /users [get]
func GetAllUser() fiber.Handler { 
    return func(c *fiber.Ctx) error {
        // Use the same collection name as auth ("users")
        collection := database.DB.Collection("users")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// bson.D{} = get all documents
		cursor, err := collection.Find(ctx, bson.D{})
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer cursor.Close(ctx)

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
                if _, ok := idIndex[uid]; !ok { continue }
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
func GetUserBy(field string) fiber.Handler {
    return func(c *fiber.Ctx) error {
        // Use the same collection name as auth ("users")
        collection := database.DB.Collection("users")
		value := c.Params("value")

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
func DeleteUser() fiber.Handler {
    return func(c *fiber.Ctx) error {
        // Only admin/root can delete users
        if !isRootByPath(viewerFrom(c)) {
            return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
        }

        id := strings.TrimSpace(c.Params("id"))
        if _, err := bson.ObjectIDFromHex(id); err != nil {
            return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid ID"})
        }

        // Delegate cascade to service
        if err := services.DeleteUserCascade(c.Context(), id); err != nil {
            return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
        }

        return c.JSON(fiber.Map{
            "success": true,
            "message": "User deleted successfully",
        })
    }
}
