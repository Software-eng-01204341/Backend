package services

import (
	"context"
	"fmt"
	"time"
	"sort"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"main-webbase/dto"
	"main-webbase/internal/models"
	repo "main-webbase/internal/repository"
)

// Use in CreateEventHandler
func CreateEventWithSchedules(body dto.EventRequestDTO, ctx context.Context) (dto.EventCreateResult, error) {
	now := time.Now().UTC()

	nodeID, err := bson.ObjectIDFromHex(body.NodeID)
	if err != nil {
		return dto.EventCreateResult{}, fmt.Errorf("invalid NodeID: %w", err)
	}
	userID, err := bson.ObjectIDFromHex(body.UserID)
	if err != nil {
		return dto.EventCreateResult{}, fmt.Errorf("invalid UserID: %w", err)
	}

	event := models.Event{
		ID:               bson.NewObjectID(),
		NodeID:           nodeID,
		UserID:           userID,
		Topic:            body.Topic,
		Description:      body.Description,
		PictureURL:       body.PictureURL,
		MaxParticipation: body.MaxParticipation,
		PostedAs:         body.PostedAs,
		Visibility:       body.Visibility,
		OrgOfContent:     body.OrgOfContent,
		Status:           body.Status,
		Have_form:        body.Have_form,
		CreatedAt:        &now,
		UpdatedAt:        &now,
	}

	// Insert event
	if err := repo.InsertEvent(ctx, event); err != nil {
		return dto.EventCreateResult{}, fmt.Errorf("failed to insert event: %w", err)
	}

	// Prepare schedules
	var schedules []models.EventSchedule
	for _, s := range body.Schedules {
		schedules = append(schedules, models.EventSchedule{
			ID:          bson.NewObjectID(),
			EventID:     event.ID,
			Date:        s.Date,
			Time_start:  s.TimeStart,
			Time_end:    s.TimeEnd,
			Location:    &s.Location,
			Description: &s.Description,
		})
	}

	if len(schedules) > 0 {
		if err := repo.InsertSchedules(ctx, schedules); err != nil {
			return dto.EventCreateResult{}, fmt.Errorf("failed to insert schedules: %w", err)
		}
	}

	var formID string
	if body.Have_form {
		form := models.Event_form{
			ID:        bson.NewObjectID(),
			Event_ID:  event.ID,
			CreatedAt: &now,
			UpdatedAt: &now,
		}

		if err := repo.InitializeForm(ctx, form); err != nil {
			return dto.EventCreateResult{}, fmt.Errorf("failed to initialize form: %w", err)
		}
		formID = form.ID.Hex()
	}

	members, err := repo.FindMembershipByManageEvent(ctx, body.OrgOfContent)
	if err != nil {
		return dto.EventCreateResult{}, fmt.Errorf("failed to find memberships: %w", err)
	}

	var participants []models.Event_participant
	for _, member := range members {
		participants = append(participants, models.Event_participant{
			ID:        bson.NewObjectID(),
			Event_ID:  event.ID,
			User_ID:   member.UserID,
			Status:    "accept",
			Role:      "organizer",
			CreatedAt: &now,
		})
	}
	if len(participants) > 0 {
		if err := repo.AddListParticipant(ctx, participants); err != nil {
			return dto.EventCreateResult{}, fmt.Errorf("failed to add initial participants: %w", err)
		}
	}

	return dto.EventCreateResult{
		Event:        event,
		Schedules:    schedules,
		FormID:       formID,
		OrganizerCnt: len(participants),
	}, nil
}

// Use in GetAllVisibleEventHandler
type VisibleEventQuery struct {
    Roles    []string
    Q        string
	Authors  []bson.ObjectID
}

func GetVisibleEventsFiltered(viewerID bson.ObjectID, ctx context.Context, userOrgSets []string, q VisibleEventQuery,) ([]dto.EventFeed, error) {
    
	var events []models.Event
	var err error
	if q.Q != "" || len(q.Roles) > 0 {
		events, err = repo.GetEventsFilter(ctx, repo.EventFilter{
			Roles: q.Roles,
			Q:     q.Q,
		})
	} else {
		events, err = repo.GetEvent(ctx)
	}
	if err != nil {
		return nil, err
	}

    // Filter visible events
    var visibleEvents []models.Event
	var eventIDs []bson.ObjectID
	for _, ev := range events {
		if CheckVisibleEvent(ctx, &ev, userOrgSets, viewerID) {
			visibleEvents = append(visibleEvents, ev)
			eventIDs = append(eventIDs, ev.ID)
		}
	}
	if len(visibleEvents) == 0 {
		return []dto.EventFeed{}, nil
	}

    // ดึง Fetch schedules
    scheds, err := repo.GetSchedulesByEvent(ctx, eventIDs)
    if err != nil {
        return nil, err
    }
    schedMap := make(map[bson.ObjectID][]models.EventSchedule)
    for _, s := range scheds {
        schedMap[s.EventID] = append(schedMap[s.EventID], s)
    }

	// Update expired events (inactive)
	now := time.Now().UTC()
	for eventID, evSchedules := range schedMap {
		var lastTime time.Time
		for _, s := range evSchedules {
			if s.Time_end.After(lastTime) {
				lastTime = s.Time_end
			}
		}

		if lastTime.Before(now) {
			err = repo.UpdateEvent(ctx, eventID, bson.M{
				"status":     "inactive",
				"updated_at": now,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to update event %s: %v", eventID.Hex(), err)
			}
			delete(schedMap, eventID)
		}
	}

	// Participant counts
	participantCounts, err := repo.GetAllEventParticipant(ctx, eventIDs)
	if err != nil {
		participantCounts = make(map[bson.ObjectID]int)
	}

	// Find next upcoming schedule per event
	type evWithNext struct {
		ev      models.Event
		next    time.Time
		nextSch models.EventSchedule
	}
	bucket := []evWithNext{}

	for _, ev := range visibleEvents {
		ss, ok := schedMap[ev.ID]
		if !ok || len(ss) == 0 {
			continue
		}

		var nextTime time.Time
		var nextRec models.EventSchedule
		var found bool

		for _, s := range ss {
			if s.Time_start.IsZero() || s.Time_start.Before(now) {
				continue
			}
			if !found || s.Time_start.Before(nextTime) {
				nextTime = s.Time_start
				nextRec = s
				found = true
			}
		}

		if found {
			bucket = append(bucket, evWithNext{
				ev:      ev,
				next:    nextTime,
				nextSch: nextRec,
			})
		}
	}

	// Sort by closest next schedule
	sort.Slice(bucket, func(i, j int) bool {
		if bucket[i].next.Equal(bucket[j].next) {
			// Tie-breaker: created_at then ID
			ti, tj := time.Time{}, time.Time{}
			if bucket[i].ev.CreatedAt != nil {
				ti = *bucket[i].ev.CreatedAt
			}
			if bucket[j].ev.CreatedAt != nil {
				tj = *bucket[j].ev.CreatedAt
			}
			if !ti.Equal(tj) {
				return ti.Before(tj)
			}
			return bucket[i].ev.ID.Hex() < bucket[j].ev.ID.Hex()
		}
		return bucket[i].next.Before(bucket[j].next)
	})

	// Build DTO for frontend
	out := make([]dto.EventFeed, 0, len(bucket))
	for _, it := range bucket {
		feed := dto.EventFeed{
			EventID:              it.ev.ID.Hex(),
			OrgPath:              it.ev.OrgOfContent,
			Topic:                it.ev.Topic,
			Description:          it.ev.Description,
			PictureURL:           it.ev.PictureURL,
			MaxParticipation:     it.ev.MaxParticipation,
			CurrentParticipation: participantCounts[it.ev.ID],
			PostedAs:             it.ev.PostedAs,
			Visibility:           it.ev.Visibility,
			Status:               it.ev.Status,
			Have_form:            it.ev.Have_form,
			Schedules:            []models.EventSchedule{it.nextSch}, // only closest
			CreatedAt:            it.ev.CreatedAt,
		}
		out = append(out, feed)
	}

	return out, nil
}

func CheckVisibleEvent(ctx context.Context, event *models.Event, userOrgs []string, userID bson.ObjectID) bool {
	if event.Status == "inactive" {
		return false
	}

	if event.Status == "active" {
		v := event.Visibility
		if v == nil {
			return false
		}
		switch v.Access {
		// 1. Access = public
		case "public":
			return true

		// 2. Access = Selected path only
		case "private":
			if len(v.Audience) == 0 {
				return false
			}

			subtreeSet := map[string]struct{}{}
			for _, s := range userOrgs {
				subtreeSet[s] = struct{}{}
			}

			for _, a := range v.Audience {
				if _, ok := subtreeSet[a.OrgPath]; ok {
					return true
				}
			}
			return false
		}
	}

	if event.Status == "draft" {
		subtreeSet := map[string]struct{}{} 
		for _, s := range userOrgs { 
			subtreeSet[s] = struct{}{} 
		} 
		if _, ok := subtreeSet[event.OrgOfContent]; ok { 
			userStatus, err := GetParticipantStatus(ctx, userID.Hex(), event.ID.Hex())
			if err != nil {
				return false
			}
			if userStatus.Role == "organizer" {
				return true
			}
		}
		return false
	}

	return false
}

func GetEventDetail(eventID string, ctx context.Context) (dto.EventDetail, error) {
	// Convert string ID to BSON ObjectID
	EventID, err := bson.ObjectIDFromHex(eventID)
	if err != nil {
		return dto.EventDetail{}, fmt.Errorf("invalid EventID: %w", err)
	}

	// Fetch event
	event, err := repo.GetEventByID(ctx, EventID)
	if err != nil {
		return dto.EventDetail{}, fmt.Errorf("failed to get event: %w", err)
	}

	// Fetch schedules
	schedules, err := repo.GetEventScheduleByID(ctx, EventID)
	if err != nil {
		return dto.EventDetail{}, fmt.Errorf("failed to get schedules: %w", err)
	}

	// Fetch user details for each participant
	userParticipants, participantCount, err := repo.GetEventParticipantsWithDetails(ctx, EventID)
	if err != nil {
		return dto.EventDetail{}, fmt.Errorf("failed to get participants: %w", err)
	}

	eventDetail := dto.EventDetail{
		EventID:              event.ID.Hex(),
		OrgPath:              event.OrgOfContent,
		Topic:                event.Topic,
		Description:          event.Description,
		PictureURL:           event.PictureURL,
		MaxParticipation:     event.MaxParticipation,
		CurrentParticipation: participantCount, 
		PostedAs:             event.PostedAs,
		Visibility:           event.Visibility,
		Status:               event.Status,
		Have_form:            event.Have_form,
		Schedules:            schedules,
		UserParticipants:     userParticipants,
	}

	return eventDetail, nil
}


func ParticipateEvent(eventID string, uid string, ctx context.Context) error {
	now := time.Now().UTC()

	eventObjID, err := bson.ObjectIDFromHex(eventID)
	if err != nil {
		return fmt.Errorf("invalid EventID: %w", err)
	}

	userObjID, err := bson.ObjectIDFromHex(uid)
	if err != nil {
		return fmt.Errorf("invalid UserID: %w", err)
	}

	event, err := repo.GetEventByID(ctx, eventObjID)
	if err != nil {
		return fmt.Errorf("failed to find event: %w", err)
	}
	if event.Have_form {
		return fmt.Errorf("this event requires form submission to join")
	}

	exists, err := repo.CheckParticipantExists(ctx, eventObjID, userObjID)
	if err != nil {
		return fmt.Errorf("failed to check participant: %w", err)
	}
	if exists {
		return fmt.Errorf("user already joined this event")
	}

	participant := models.Event_participant{
		ID:        bson.NewObjectID(),
		Event_ID:  eventObjID,
		User_ID:   userObjID,
		Status:    "accept",
		Role:      "participant",
		CreatedAt: &now,
	}

	if err := repo.AddParticipant(ctx, participant); err != nil {
		return fmt.Errorf("failed to add participant: %w", err)
	}

	return nil
}

func UpdateEventWithSchedules(eventID bson.ObjectID, body dto.EventRequestDTO, ctx context.Context) (dto.EventRequestDTO, error) {
	now := time.Now().UTC()

	nodeID, err := bson.ObjectIDFromHex(body.NodeID)
	if err != nil {
		return dto.EventRequestDTO{}, fmt.Errorf("invalid NodeID: %w", err)
	}

	eventUpdates := bson.M{
		"node_id":           nodeID,
		"topic":             body.Topic,
		"description":       body.Description,
		"picture_url":       body.PictureURL,
		"max_participation": body.MaxParticipation,
		"posted_as":         body.PostedAs,
		"visibility":        body.Visibility,
		"org_of_content":    body.OrgOfContent,
		"status":            body.Status,
		"have_form":         body.Have_form,
		"updated_at":        now,
	}

	if err := repo.UpdateEvent(ctx, eventID, eventUpdates); err != nil {
		return dto.EventRequestDTO{}, fmt.Errorf("failed to update event: %w", err)
	}

	// Delete old schedules
	if err := repo.DeleteEventScheduleByID(ctx, eventID); err != nil {
		return dto.EventRequestDTO{}, fmt.Errorf("failed to delete old schedules: %w", err)
	}

	// Prepare schedules
	var schedules []models.EventSchedule
	for _, s := range body.Schedules {
		schedules = append(schedules, models.EventSchedule{
			ID:          bson.NewObjectID(),
			EventID:     eventID,
			Date:        s.Date,
			Time_start:  s.TimeStart,
			Time_end:    s.TimeEnd,
			Location:    &s.Location,
			Description: &s.Description,
		})
	}

	if len(schedules) > 0 {
		if err := repo.InsertSchedules(ctx, schedules); err != nil {
			return dto.EventRequestDTO{}, fmt.Errorf("failed to insert schedules: %w", err)
		}
	}

	if body.Have_form {
		form, err := repo.FindFormByEventID(ctx, eventID.Hex())
		if err != nil && err != mongo.ErrNoDocuments {
			return dto.EventRequestDTO{}, fmt.Errorf("failed to fetch form: %w", err)
		}

		if form == nil {
			newForm := models.Event_form{
				ID:        bson.NewObjectID(),
				Event_ID:  eventID,
				CreatedAt: &now,
				UpdatedAt: &now,
			}
			if err := repo.InitializeForm(ctx, newForm); err != nil {
				return dto.EventRequestDTO{}, fmt.Errorf("failed to initialize form: %w", err)
			}
		}
	}

	return body, err
}
