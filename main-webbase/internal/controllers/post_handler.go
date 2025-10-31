package controllers

import (
	"context"
	"errors"
	"path/filepath"

	"fmt"
	"main-webbase/dto"
	"main-webbase/internal/accessctx"
	mid "main-webbase/internal/middleware"
	repo "main-webbase/internal/repository"
	"main-webbase/internal/services"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func canPostAs(v *accessctx.ViewerAccess, orgPath, positionKey string) bool {
	if v == nil {
		return false
	}
	// root/admin (OrgPath == "/") ใช้ได้ทุกอย่าง
	if isRootByPath(v) {
		return true
	}
	// ต้องมี membership ที่ตรงทั้ง org_path และ position_key
	for _, m := range v.Memberships {
		// หมายเหตุ: field ชื่อ PosKey ใน viewer
		if m.OrgPath == orgPath && m.PosKey == positionKey {
			return true
		}
	}
	return false
}

const serverIP = "45.144.166.252:46602"

// POST /posts

// CreatePostHandler godoc
// @Summary Create a post with optional media upload
// @Description Create a new post; supports multipart form upload
// @Tags posts
// @Accept multipart/form-data
// @Produce json
// @Param file formData file false "Upload media file"
// @Param postText formData string true "Post text"
// @Param org_of_content formData string false "Organization of content"
// @Param postAs.org_path formData string true "Organization path"
// @Param postAs.position_key formData string true "Position key"
// @Param visibility.access formData string false "Visibility (default: public)"
// @Param categoryIds formData string false "Category IDs (repeatable)"
// @Success 201 {object} dto.PostResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Router /posts [post]
func CreatePostHandler(client *mongo.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID, _ := mid.UIDFromLocals(c)

		var body dto.CreatePostDTO
		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).
				JSON(dto.ErrorResponse{Error: "invalid body"})
		}

		// --- manually read nested postAs fields ---
		postAsOrgPath := c.FormValue("postAs.org_path")
		postAsPosition := c.FormValue("postAs.position_key")
		if postAsOrgPath == "" {
			postAsOrgPath = body.PostAs.OrgPath
		}
		if postAsPosition == "" {
			postAsPosition = body.PostAs.PositionKey
		}
		if postAsOrgPath == "" || postAsPosition == "" {
			return c.Status(fiber.StatusBadRequest).
				JSON(dto.ErrorResponse{Error: "postAs.org_path and postAs.position_key are required"})
		}
		body.PostAs.OrgPath = postAsOrgPath
		body.PostAs.PositionKey = postAsPosition
		// ---------------------------------------------------------------

		// --- handle optional file upload ---
		file, err := c.FormFile("file")
		if err == nil && file != nil {
			timestamp := time.Now().UnixNano() / 1e6
			ext := filepath.Ext(file.Filename)
			filename := fmt.Sprintf("%s_%d%s", userID, timestamp, ext)
			savePath := filepath.Join("/var/www/html/uploads", filename)

			if err := c.SaveFile(file, savePath); err != nil {
				return c.Status(fiber.StatusInternalServerError).
					JSON(dto.ErrorResponse{Error: "failed to save file"})
			}

			// Add public URL yourto MongoDB
			publicURL := fmt.Sprintf("http://%s/uploads/%s", serverIP, filename)
			body.Media = append(body.Media, publicURL)
		}
		// ---------------------------------------------------------------

		if body.PostText == "" {
			return c.Status(fiber.StatusBadRequest).
				JSON(dto.ErrorResponse{Error: "postText is required"})
		}

		if !canPostAs(viewerFrom(c), body.PostAs.OrgPath, body.PostAs.PositionKey) {
			return c.Status(fiber.StatusForbidden).
				JSON(dto.ErrorResponse{Error: "forbidden: you cannot post as this role"})
		}

		if body.Visibility.Access == "" {
			body.Visibility.Access = "public"
		}

		ctx := context.Background()
		post, err := services.CreatePostWithMeta(client, userID, body, ctx)
		if err != nil {
			switch {
			case errors.Is(err, services.ErrUserNotFound):
				return c.Status(fiber.StatusNotFound).JSON(dto.ErrorResponse{Error: "user not found"})
			case errors.Is(err, services.ErrOrgNodeNotFound):
				return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{Error: "org_path not found"})
			case errors.Is(err, services.ErrPositionNotFound):
				return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{Error: "position_key not found"})
			default:
				return c.Status(fiber.StatusInternalServerError).JSON(dto.ErrorResponse{Error: err.Error()})
			}
		}

		return c.Status(fiber.StatusCreated).JSON(post)
	}
}

// GET /posts/:post_id

// GetIndividualPostHandler godoc
// @Summary      Get a post detail
// @Description  Return post detail (user, position, org path, visibility, categories, likes count, etc.)
// @Tags         posts
// @Accept       json
// @Produce      json
// @Param        post_id  path  string  true  "Post ID (hex)"
// @Success      200  {object}  dto.PostResponse
// @Failure      400  {object}  dto.ErrorResponse
// @Failure      404  {object}  dto.ErrorResponse
// @Failure      500  {object}  dto.ErrorResponse
// @Router       /posts/{post_id} [get]
func GetIndividualPostHandler(client *mongo.Client) fiber.Handler {
	const dbName = "unicom"

	return func(c *fiber.Ctx) error {

		postIDHex := c.Params("post_id")
		userID, _ := mid.UIDObjectID(c)

		if postIDHex == "" {
			return fiber.NewError(fiber.StatusBadRequest, "missing post_id in route")
		}
		postID, err := bson.ObjectIDFromHex(postIDHex)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid post_id")
		}

		// context พร้อม timeout
		ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
		defer cancel()

		db := client.Database(dbName)

		resp, err := services.GetPostDetail(ctx, db, userID, postID)
		if err != nil {
			// ถ้าถูก wrap ด้วย %w จาก service จะเช็ค ErrNoDocuments ได้
			if errors.Is(err, mongo.ErrNoDocuments) {
				return fiber.NewError(fiber.StatusNotFound, "post not found")
			}
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		return c.Status(fiber.StatusOK).JSON(resp)
	}
}

// DeletePostHandlerWithClient สร้าง Fiber handler โดยรับ mongo.Client + dbName
// @Summary      Soft delete post (status: active -> inactive)
// @Description  เปลี่ยนสถานะโพสต์จาก active เป็น inactive (soft delete)
// @Tags         posts
// @Param        id   path      string  true  "Post ID (hex)"
// @Success      204
// @Failure      400  {object}  dto.ErrorResponse
// @Failure      404  {object}  dto.ErrorResponse
// @Failure      500  {object}  dto.ErrorResponse
// @Router       /posts/{post_id} [delete]
func DeletePostHandler(client *mongo.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		uid, _ := mid.UIDObjectID(c)

		idParam := c.Params("post_id")
		postID, err := bson.ObjectIDFromHex(idParam)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid post id")
		}

		isRoot := isRootByPath(viewerFrom(c))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		db := client.Database("unicom")

		// Optional hard delete for admin/root via ?hard=true
		hard := strings.EqualFold(strings.TrimSpace(c.Query("hard")), "true")
		if hard {
			if !isRoot {
				return fiber.NewError(fiber.StatusForbidden, "forbidden")
			}
			ok, err := repo.DeletePostCascade(db, postID, ctx)
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
			if !ok {
				return fiber.NewError(fiber.StatusNotFound, "post not found")
			}
			return c.SendStatus(fiber.StatusNoContent)
		}

		ok, err := repo.DeletePost(db, postID, ctx, uid, isRoot)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if !ok {
			// ไม่พบ/ไม่ active หรือไม่มีสิทธิ์ (ไม่ใช่เจ้าของและไม่ใช่ root)
			return fiber.NewError(fiber.StatusForbidden, "forbidden")
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
}

// PUT /posts/:id

// UpdatePostHandler godoc
// @Summary      Update a post (full replace of editable fields)
// @Description  Update post text, medias, categories, visibility, and posted-as. Owner can edit content; admin/root can also change status.
// @Tags         posts
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        Authorization  header    string                   true  "Bearer {token}"
// @Param        id             path      string                   true  "Post ID (hex)"
// @Param        data           body      dto.UpdatePostFullDTO    true  "Full update payload"
// @Success      200            {object}  models.Post
// @Failure      400            {object}  dto.ErrorResponse
// @Failure      401            {object}  dto.ErrorResponse
// @Failure      403            {object}  dto.ErrorResponse
// @Failure      500            {object}  dto.ErrorResponse
// @Router       /posts/{post_id} [put]
func UpdatePostHandler(client *mongo.Client) fiber.Handler {
	return func(c *fiber.Ctx) error {
		uid, _ := mid.UIDObjectID(c)
		fmt.Printf("[Handler] uid=%s\n", uid.Hex())
		postID, err := bson.ObjectIDFromHex(c.Params("post_id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid post id")
		}
		v := viewerFrom(c)
		// b, _ := json.MarshalIndent(v, "", "  ")
		// fmt.Println("[DEBUG] Viewer =", string(b))
		var body dto.UpdatePostFullDTO
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid body")
		}
		if strings.TrimSpace(body.PostText) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "postText required")
		}

		isRoot := isRootByPath(viewerFrom(c)) // root/admin = OrgPath == "/"

		// ✅ เช็คสิทธิ์ postAs ถ้าส่งมาแก้ (เรา require postAs ใน DTO อยู่แล้ว)
		// ถ้าอยากให้ "ไม่บังคับส่ง postAs ทุกครั้ง" ให้เช็คเฉพาะกรณีที่มีค่าใหม่
		if !canPostAs(v, body.PostAs.OrgPath, body.PostAs.PositionKey) {
			return c.Status(fiber.StatusForbidden).
				JSON(dto.ErrorResponse{Error: "forbidden: you cannot post as this role2"})
		}

		ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
		defer cancel()

		db := client.Database("unicom")
		if _, err := services.UpdatePostFull(client, db, postID, uid, isRoot, body, ctx); err != nil {
			msg := err.Error()
			switch {
			case strings.Contains(msg, "forbidden"):
				// fmt.Println("[FORBIDDEN-3] handler caught forbidden:", err)
				return c.Status(403).JSON(dto.ErrorResponse{Error: "forbidden"})
			case strings.Contains(msg, "invalid") || strings.Contains(msg, "org_path not found2") || strings.Contains(msg, "position_key not found"):
				return c.Status(400).JSON(dto.ErrorResponse{Error: msg})
			case strings.Contains(msg, "post not found"):
				return c.Status(404).JSON(dto.ErrorResponse{Error: msg})
			default:
				return c.Status(500).JSON(dto.ErrorResponse{Error: msg})
			}
		}

		resp, err := services.GetPostDetail(ctx, client.Database("unicom"), uid, postID)
		if err != nil {
			return c.Status(500).JSON(dto.ErrorResponse{Error: err.Error()})
		}
		return c.Status(200).JSON(resp)
	}
}
