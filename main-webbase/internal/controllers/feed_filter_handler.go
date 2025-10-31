// internal/controllers/feed_controller.go
package controllers

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"main-webbase/internal/accessctx"
	"main-webbase/internal/models"
	"main-webbase/internal/repository"
)
// FeedHandler godoc
// @Summary List feed posts
// @Description ดึงรายการโพสต์ที่ผู้ใช้ปัจจุบันมองเห็นได้ พร้อมกรอง/จัดเรียงและแบ่งหน้า
// @Description พารามิเตอร์ที่รองรับ:
// @Description  - `q` (string): คำค้นหาแบบ full-text ในข้อความโพสต์
// @Description  - `role` (string, comma-separated): รายการบทบาทเพื่อกรอง เช่น `teacher,student`
// @Description  - `category` (string, comma-separated): รายการหมวดหมู่เพื่อกรอง เช่น `announcement,event`
// @Description  - `author` (string, comma-separated): รายการ ObjectID (hex 24) ของผู้เขียน
// @Description  - `limit` (int): จำนวนต่อหน้า 1–20 (ดีฟอลต์ 20)
// @Description  - `cursor` (string): ObjectID (hex 24) สำหรับทำ cursor pagination
// @Description  - `sort` (string): โหมดเรียงลำดับ `time` (ค่าเริ่มต้น) หรือ `popular`
// @Tags feed
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param q query string false "Search text (full-text)"
// @Param role query string false "CSV roles filter e.g. teacher,student"
// @Param category query string false "CSV categories filter e.g. announcement,event"
// @Param author query string false "CSV author ObjectIDs (hex24)"
// @Param limit query int false "Items per page (1-20). Default 20"
// @Param cursor query string false "Cursor (Mongo ObjectID hex24)"
// @Param sort query string false "Sort mode: time | popular" Enums(time, popular) default(time)
// @Success 200 {object} map[string]interface{} "items ([]Post-like), next_cursor (hex24)"
// @Failure 401 {object} map[string]string "Unauthorized - user not authenticated"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /feed [get]

// ใช้แบบ: posts.Get("/feed", controllers.FeedHandler(d.Client))
func FeedHandler(client *mongo.Client) fiber.Handler {
	repo := repository.NewMongoFeedRepo(client)

	return func(c *fiber.Ctx) error {
		limit, _ := strconv.ParseInt(c.Query("limit", "20"), 10, 64)
		if limit <= 0 {
			limit = 20
		}
		if limit > 20 {
			limit = 20
		}

		var until bson.ObjectID
		if cur := c.Query("cursor"); cur != "" {
			if oid, err := bson.ObjectIDFromHex(cur); err == nil {
				until = oid
			}
		}

		// โหมดเรียง: "popular" หรือ "time" (ค่าเริ่มต้น)
		sortBy := strings.ToLower(strings.TrimSpace(c.Query("sort", "time")))

		// ✅ ใช้ viewer จาก middleware (JWT + InjectViewer)
		vAny := c.Locals("viewer")
		if vAny == nil {
			return fiber.ErrUnauthorized
		}
		viewer, ok := vAny.(*accessctx.ViewerAccess)
		if !ok || viewer == nil {
			return fiber.NewError(fiber.StatusInternalServerError, "viewer context broken")
		}

		// ✅ พยายามดึง user_id จาก Locals (ถ้า middleware อื่นๆ ใส่มาให้)
		viewerID, _ := userIDFromLocals(c)

		opts := models.QueryOptions{
			Roles:          splitCSV(c.Query("role")),
			Categories:     splitCSV(c.Query("category")),
			AuthorIDs:      parseAuthorIDs(splitCSV(c.Query("author"))),
			TextSearch:     c.Query("q"),
			Limit:          limit,
			UntilID:        until,
			ViewerID:       viewerID,              // อาจเป็น zero value ถ้าไม่มีใน Locals
			AllowedNodeIDs: viewer.SubtreeNodeIDs, // ใช้จาก ViewerAccess รุ่นใหม่
		}

		var (
			items []models.Post
			next  *bson.ObjectID
			err   error
		)

		// ✅ เรียก popular หรือ time ตามพารามิเตอร์
		if sortBy == "popular" {
			items, next, err = repo.ListPopular(c.Context(), opts)
		} else {
			items, next, err = repo.List(c.Context(), opts)
		}
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		resp := fiber.Map{"items": items}
		if next != nil {
			resp["next_cursor"] = next.Hex()
		}
		return c.JSON(resp)
	}
}

// ===== helpers =====

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseAuthorIDs(ids []string) []bson.ObjectID {
	if len(ids) == 0 {
		return nil
	}
	out := make([]bson.ObjectID, 0, len(ids))
	for _, h := range ids {
		if oid, err := bson.ObjectIDFromHex(strings.TrimSpace(h)); err == nil {
			out = append(out, oid)
		}
	}
	return out
}

// พยายามอ่าน user_id จาก Locals ได้ทั้งแบบ bson.ObjectID และ string hex
func userIDFromLocals(c *fiber.Ctx) (bson.ObjectID, bool) {
	// 1) ถ้า Locals เก็บไว้เป็น bson.ObjectID โดยตรง
	if v := c.Locals("user_id"); v != nil {
		switch t := v.(type) {
		case bson.ObjectID:
			return t, true
		case string:
			if oid, err := bson.ObjectIDFromHex(strings.TrimSpace(t)); err == nil {
				return oid, true
			}
		}
	}
	// 2) เผื่อ middleware อื่นใช้ key อื่น เช่น "uid"
	if v := c.Locals("uid"); v != nil {
		if s, ok := v.(string); ok {
			if oid, err := bson.ObjectIDFromHex(strings.TrimSpace(s)); err == nil {
				return oid, true
			}
		}
	}
	return bson.ObjectID{}, false
}
