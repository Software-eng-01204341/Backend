package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"main-webbase/database"
	"main-webbase/dto"
	"main-webbase/internal/middleware"
	"main-webbase/internal/models"
	"main-webbase/internal/repository"
	"main-webbase/internal/services"
)

// CreateEventHandler godoc
// @Summary Create a new event
// @Description Create an event with optional image upload and schedule creation
// @Tags events
// @Accept multipart/form-data
// @Produce json
// @Param file formData file false "Event image file upload"
// @Param NodeID formData string true "Node ID"
// @Param topic formData string false "Event topic"
// @Param description formData string false "Event description"
// @Param org_of_content formData string false "Organization responsible for content (e.g. /fac/eng/com)"
// @Param status formData string false "Event status" Enums(active, draft, inactive)
// @Param max_participation formData int false "Maximum participants"
// @Param have_form formData bool false "Whether event has a form (true/false)"
// @Param postedAs.org_path formData string true "Organization path of the posting role"
// @Param postedAs.position_key formData string true "Position key of the posting role"
// @Param visibility formData string false "Visibility JSON string, e.g., {\"access\":\"public\"}"
// @Param schedules formData string false "Schedules JSON array, e.g., [{\"date\":\"2025-10-15T00:00:00Z\",\"time_start\":\"2025-10-15T09:00:00Z\",\"time_end\":\"2025-10-15T12:00:00Z\",\"location\":\"Room 301\",\"description\":\"Morning session\"}]"
// @Success 201 {object} dto.EventCreateResult "Event created successfully"
// @Failure 400 {object} map[string]string "Bad request (invalid input)"
// @Failure 403 {object} dto.ErrorResponse "Forbidden: Cannot post as this role"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /event [post]
func CreateEventHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		var body dto.EventRequestDTO

		// Initialize nested pointers first to avoid nil deref
		if body.PostedAs == nil {
			body.PostedAs = &models.PostedAs{}
		}

		// Parse minimal required form fields
		body.NodeID = c.FormValue("NodeID")
		body.PostedAs.OrgPath = c.FormValue("postedAs.org_path")
		body.PostedAs.PositionKey = c.FormValue("postedAs.position_key")

		// Optional basic fields
		if v := c.FormValue("topic"); v != "" {
			body.Topic = v
		}
		if v := c.FormValue("description"); v != "" {
			body.Description = v
		}
		if v := c.FormValue("org_of_content"); v != "" {
			body.OrgOfContent = v
		}
		if v := c.FormValue("status"); v != "" {
			body.Status = v
		}
		if v := c.FormValue("max_participation"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				body.MaxParticipation = n
			}
		}
		if v := c.FormValue("have_form"); v != "" {
			switch v {
			case "1", "true", "TRUE", "True":
				body.Have_form = true
			default:
				body.Have_form = false
			}
		}

		// Visibility (JSON string in multipart form)
		if v := c.FormValue("visibility"); v != "" {
			var vis models.Visibility
			if err := json.Unmarshal([]byte(v), &vis); err == nil {
				body.Visibility = &vis
			}
		}
		// Safe default to avoid nil visibility panics in service layer
		if body.Visibility == nil {
			body.Visibility = &models.Visibility{Access: "public"}
		}

		if body.NodeID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "NodeID is required"})
		}
		if body.PostedAs.OrgPath == "" || body.PostedAs.PositionKey == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "postedAs.org_path and postedAs.position_key are required"})
		}

		// --- optional file upload ---
		file, err := c.FormFile("file")
		if err == nil && file != nil {
			timestamp := time.Now().UnixNano() / 1e6
			ext := filepath.Ext(file.Filename)
			filename := fmt.Sprintf("event_%d%s", timestamp, ext)
			savePath := filepath.Join("/var/www/html/uploads", filename)

			if err := c.SaveFile(file, savePath); err != nil {
				return c.Status(fiber.StatusInternalServerError).
					JSON(fiber.Map{"error": "failed to save file"})
			}

			publicURL := fmt.Sprintf("http://%s/uploads/%s", serverIP, filename)

			// Persist uploaded image URL into request DTO so it is stored with the event
			body.PictureURL = &publicURL
		}

		// --- permission check ---
		if !canPostAs(viewerFrom(c), body.PostedAs.OrgPath, body.PostedAs.PositionKey) {
			return c.Status(fiber.StatusForbidden).
				JSON(dto.ErrorResponse{Error: "forbidden: you cannot post as this role"})
		}

		// --- create event ---
		result, err := services.CreateEventWithSchedules(body, c.Context())
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		// Optionally append image URL in response
		if imgURL := body.PictureURL; imgURL != nil {
			return c.Status(fiber.StatusCreated).JSON(fiber.Map{
				"result":    result,
				"image_url": imgURL,
			})
		}

		return c.Status(fiber.StatusCreated).JSON(result)
	}
}

// GetAllVisibleEventHandler godoc
// @Summary Get all visible events
// @Description Retrieve all events that the current user can see. This endpoint returns events visible to the current authenticated user. You can optionally filter the events using query parameters: \n - `q` (string): Search text that matches the event's topic or description (case-insensitive). \n - `role` (string, comma-separated): Filter by user role or organization path. Each value can be: \n - A position key, e.g., `Lecturer` (matches `postedas.position_key`, case-insensitive exact match). \n - An organization path, e.g., `/fac/eng/com` (matches `org_of_content`). \n - Subtree prefix is supported using `/*`, e.g., `/fac/eng/*` matches `/fac/eng/com` or `/fac/eng/math`. \n If no filters are applied, all events that the user can see (based on visibility rules) are returned. \n Visibility rules: \n - `public`: visible to everyone. \n - `org`: visible only to users whose organization is included in the audience. \n - `draft`: visible only to organizers within the same organization.
// @Tags events
// @Accept json
// @Produce json
// @Param q query string false "Search text in topic or description"
// @Param role query string false "Comma-separated list of roles or org paths to filter"
// @Success 200 {array} map[string]interface{} "Array of visible events with their next upcoming schedule"
// @Failure 401 {object} map[string]string "Unauthorized - user not authenticated"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /event [get]
func GetAllVisibleEventHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		viewerID, err := services.UserIDFrom(c)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
		}
		log.Printf("[DEBUG] viewerID=%s", viewerID.Hex())

		orgSets, err := services.AllUserOrg(viewerID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get user orgs"})
		}
		log.Printf("[DEBUG] orgSets=%+v", orgSets)

		// ---------- parse filters ----------
		q := c.Query("q")
		roles := splitCSVFilter(c.Query("role"))

		// events, err := services.GetVisibleEvents(viewerID, ctx, orgSets)
		events, err := services.GetVisibleEventsFiltered(
			viewerID,
			ctx,
			orgSets,
			services.VisibleEventQuery{
				Roles: roles,
				Q:     q,
			},
		)

		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(events)
	}
}

// GetAllPendingEventHandler godoc
// @Summary      List pending events
// @Description  Return every event that is still pending approval, sorted by newest first.
// @Tags         events
// @Produce      json
// @Success      200  {array}  dto.EventDetail
// @Failure      500  {object} map[string]string "Internal server error"
// @Router       /event/pending [get]
func GetAllPendingEventHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		col := database.DB.Collection("events") // ใช้ global DB ตามที่คุณใช้อยู่

		items, err := services.QueryPendingEvents(
			c.Context(),
			col,
		)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(items)
	}
}

func splitCSVFilter(s string) []string {
	if s == "" {
		return []string{}
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

// GetEventDetailHandler godoc
// @Summary Get event detail
// @Description Retrieve full details of an event including schedules and form ID
// @Tags events
// @Produce json
// @Param event_id path string true "Event ID"
// @Success 200 {object} dto.EventDetail
// @Failure 400 {object} map[string]string "Invalid request"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /event/{event_id} [get]
func GetEventDetailHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		eventID := c.Params("event_id")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		event, err := services.GetEventDetail(eventID, ctx)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.JSON(event)
	}
}

// ParticipateEventWithNoFormHandler godoc
// @Summary Participate in an event (no form required)
// @Description Join an event directly if it does not require a form
// @Tags events
// @Param event_id path string true "Event ID"
// @Produce json
// @Success 200 {object} map[string]string "Participation successful"
// @Failure 400 {object} map[string]string "Bad request"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /event/participate/{event_id} [post]
func ParticipateEventWithNoFormHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		eventID := c.Params("event_id")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		uid, err := middleware.UIDFromLocals(c)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
		}

		if err = services.ParticipateEvent(eventID, uid, ctx); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"message":  "Participation successful",
			"event_id": eventID,
			"user_id":  uid,
		})
	}
}

// DeleteEventHandler godoc
// @Summary Soft delete an event
// @Description Mark event as hidden (status=inactive)
// @Tags events
// @Param event_id path string true "Event ID"
// @Success 200 {object} map[string]string "Event deleted successfully"
// @Failure 400 {object} map[string]string "Invalid ID"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /event/{event_id} [delete]
func DeleteEventHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		collection_event := database.DB.Collection("events")

		idHex := c.Params("event_id")
		eventID, err := bson.ObjectIDFromHex(idHex)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid ID"})
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err = collection_event.UpdateOne(ctx, bson.M{"_id": eventID},
			bson.M{"$set": bson.M{"status": "inactive", "updated_at": time.Now()}})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete event"})
		}

		// Notify participants about event deletion
		// parameter for notification
		userIds, err := repository.FindAcceptedParticipants(ctx, eventID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get participants"})
		}

		ref := models.Ref{
			ID:     eventID,
			Entity: "event",
		}
		colEvent := database.DB.Collection("events")
		colNoti := database.DB.Collection("notification")
		var result struct {
			Title string `bson:"topic"`
		}
		err = colEvent.FindOne(c.Context(), bson.M{"_id": eventID}).Decode(&result)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to fetch event title: "+err.Error())
		}

		notiParam := models.NotiParams{
			EventTitle: result.Title,
			EventID:    eventID,
		}
		if err := services.NotifyMany(c.Context(),
			colNoti,
			userIds,
			services.NotiEventDeleted,
			ref,
			notiParam); err != nil {
			return err
		}

		return c.JSON(fiber.Map{"message": "Event deleted (soft)"})
	}
}

// UpdateEventHandler godoc
// @Summary Update an event
// @Description Update any field of an event, including image and schedules
// @Tags events
// @Accept json,multipart/form-data
// @Produce json
// @Param event_id path string true "Event ID"
// @Param body body dto.EventRequestDTO true "Event fields to update (JSON)"
// @Param file formData file false "Optional event image file"
// @Success 200 {object} dto.EventRequestDTO "Event updated successfully"
// @Failure 400 {object} map[string]string "Bad request (invalid event_id or request body)"
// @Failure 403 {object} dto.ErrorResponse "Forbidden: Cannot post as this role"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /event/{event_id} [patch]
func UpdateEventHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		eventIDHex := c.Params("event_id")
		eventID, err := bson.ObjectIDFromHex(eventIDHex)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid Event ID"})
		}

		var body dto.EventRequestDTO

		// Initialize nested pointers early
		if body.PostedAs == nil {
			body.PostedAs = &models.PostedAs{}
		}

		// Parse form values (multipart form)
		body.NodeID = c.FormValue("NodeID")
		body.PostedAs.OrgPath = c.FormValue("postedAs.org_path")
		body.PostedAs.PositionKey = c.FormValue("postedAs.position_key")

		if v := c.FormValue("topic"); v != "" {
			body.Topic = v
		}
		if v := c.FormValue("description"); v != "" {
			body.Description = v
		}
		if v := c.FormValue("org_of_content"); v != "" {
			body.OrgOfContent = v
		}
		if v := c.FormValue("status"); v != "" {
			body.Status = v
		}
		if v := c.FormValue("max_participation"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				body.MaxParticipation = n
			}
		}
		if v := c.FormValue("have_form"); v != "" {
			switch v {
			case "1", "true", "TRUE", "True":
				body.Have_form = true
			default:
				body.Have_form = false
			}
		}

		// Visibility (JSON string in multipart form)
		if v := c.FormValue("visibility"); v != "" {
			var vis models.Visibility
			if err := json.Unmarshal([]byte(v), &vis); err == nil {
				body.Visibility = &vis
			}
		}
		if body.Visibility == nil {
			body.Visibility = &models.Visibility{Access: "public"}
		}

		// Optional file upload
		file, err := c.FormFile("file")
		if err == nil && file != nil {
			timestamp := time.Now().UnixNano() / 1e6
			ext := filepath.Ext(file.Filename)
			filename := fmt.Sprintf("event_%d%s", timestamp, ext)
			savePath := filepath.Join("/var/www/html/uploads", filename)

			if err := c.SaveFile(file, savePath); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to save file"})
			}

			publicURL := fmt.Sprintf("http://%s/uploads/%s", serverIP, filename)
			body.PictureURL = &publicURL
		}

		// --- validation ---
		if body.NodeID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "NodeID is required"})
		}
		// if body.PostedAs.OrgPath == "" || body.PostedAs.PositionKey == "" {
		// 	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "postedAs.org_path and postedAs.position_key are required"})
		// }

		// --- permission check ---
		if !canPostAs(viewerFrom(c), body.PostedAs.OrgPath, body.PostedAs.PositionKey) {
			return c.Status(fiber.StatusForbidden).JSON(dto.ErrorResponse{Error: "forbidden: you cannot post as this role"})
		}

		// --- update event ---
		result, err := services.UpdateEventWithSchedules(eventID, body, c.Context())
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		// Append image URL if uploaded
		if imgURL := body.PictureURL; imgURL != nil {
			return c.Status(fiber.StatusOK).JSON(fiber.Map{
				"result":    result,
				"image_url": imgURL,
			})
		}

		// Notify participants about event update
		// parameter for notification
		userIds, err := repository.FindAcceptedParticipants(c.Context(), eventID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to get participants"})
		}

		ref := models.Ref{
			ID:     eventID,
			Entity: "event",
		}
		colEvent := database.DB.Collection("events")
		colNoti := database.DB.Collection("notification")
		var doc struct {
			Title string `bson:"topic"`
		}
		err = colEvent.FindOne(c.Context(), bson.M{"_id": eventID}).Decode(&result)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to fetch event title: "+err.Error())
		}

		notiParam := models.NotiParams{
			EventTitle: doc.Title,
			EventID:    eventID,
		}
		if err := services.NotifyMany(c.Context(),
			colNoti,
			userIds,
			services.NotiEventUpdated,
			ref,
			notiParam); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to send notification")
		}

		return c.Status(fiber.StatusOK).JSON(result)
	}
}

// ให้ error format เหมือนเดิมทุกที่
func writeErr(c *fiber.Ctx, code int, msg string) error {
	return c.Status(code).JSON(dto.ErrorResponse{Error: msg})
}

// PatchAcceptEventHandler godoc
// @Summary      Approve a pending event
// @Description  Mark a pending event as active.
// @Tags         events
// @Param        event_id  path  string  true  "Event ID"
// @Success      200  {object} dto.EventDetail
// @Failure      400  {object} dto.ErrorResponse "Invalid Event ID"
// @Failure      404  {object} dto.ErrorResponse "pending event not found or already processed"
// @Failure      500  {object} dto.ErrorResponse "Internal server error"
// @Router       /event/accept/{event_id} [patch]
func PatchAcceptEventHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		col := database.DB.Collection("events")

		idHex := c.Params("event_id")
		oid, err := bson.ObjectIDFromHex(idHex)
		if err != nil {
			return writeErr(c, fiber.StatusBadRequest, "Invalid Event ID")
		}

		ctx, cancel := context.WithTimeout(c.Context(), 8*time.Second)
		defer cancel()

		now := time.Now().UTC()
		opt := options.FindOneAndUpdate().SetReturnDocument(options.After)

		res := col.FindOneAndUpdate(
			ctx,
			bson.M{"_id": oid, "status": "pending"},
			bson.M{"$set": bson.M{
				"status":     "active",
				"updated_at": now,
			}},
			opt,
		)

		if err := res.Err(); err != nil {
			if err == mongo.ErrNoDocuments {
				return writeErr(c, fiber.StatusNotFound, "pending event not found or already processed")
			}
			return writeErr(c, fiber.StatusInternalServerError, err.Error())
		}

		var ev dto.EventDetail
		if err := res.Decode(&ev); err != nil {
			return writeErr(c, fiber.StatusInternalServerError, err.Error())
		}

		return c.Status(fiber.StatusOK).JSON(ev)
	}
}


// PatchRejectEventHandler godoc
// @Summary      Reject a pending event
// @Description  Mark a pending event as rejected.
// @Tags         events
// @Param        event_id  path  string  true  "Event ID"
// @Success      200  {object} dto.EventDetail
// @Failure      400  {object} dto.ErrorResponse "Invalid Event ID"
// @Failure      404  {object} dto.ErrorResponse "pending event not found or already processed"
// @Failure      500  {object} dto.ErrorResponse "Internal server error"
// @Router       /event/reject/{event_id} [patch]
func PatchRejectEventHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		col := database.DB.Collection("events")

		idHex := c.Params("event_id")
		oid, err := bson.ObjectIDFromHex(idHex)
		if err != nil {
			return writeErr(c, fiber.StatusBadRequest, "Invalid Event ID")
		}

		ctx, cancel := context.WithTimeout(c.Context(), 8*time.Second)
		defer cancel()

		update := bson.M{
			"status":     "rejected",
			"updated_at": time.Now().UTC(),
		}

		opt := options.FindOneAndUpdate().SetReturnDocument(options.After)

		res := col.FindOneAndUpdate(
			ctx,
			bson.M{"_id": oid, "status": "pending"},
			bson.M{"$set": update},
			opt,
		)

		if err := res.Err(); err != nil {
			if err == mongo.ErrNoDocuments {
				return writeErr(c, fiber.StatusNotFound, "pending event not found or already processed")
			}
			return writeErr(c, fiber.StatusInternalServerError, err.Error())
		}

		var ev dto.EventDetail
		if err := res.Decode(&ev); err != nil {
			return writeErr(c, fiber.StatusInternalServerError, err.Error())
		}

		return c.Status(fiber.StatusOK).JSON(ev)
	}
}
