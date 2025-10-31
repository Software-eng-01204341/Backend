package dto

import (
	"main-webbase/internal/models"
	"time"
)

// Send Everything about event
type EventRequestDTO struct {
	NodeID           string             `json:"node_id" example:"66ffa43e9a7c39b1d87f6401" validate:"required"`
	UserID           string             `json:"user_id" example:"68e4e0ff6c65869959678b9f" validate:"required"`
	Topic            string             `json:"topic" example:"AI Workshop" validate:"required"`
	PictureURL       *string             `json:"picture_url,omitempty" example:"http://45.144.166.252:46602/uploads/cat.png"`
	Description      string             `json:"description" example:"A workshop on AI applications"`
	MaxParticipation int                `json:"max_participation" example:"50"`
	PostedAs         *models.PostedAs   `json:"posted_as,omitempty"`
	Visibility       *models.Visibility `json:"visibility,omitempty"`
	OrgOfContent     string             `json:"org_of_content,omitempty" example:"/fac/eng/com"`
	Status           string             `json:"status" example:"draft" enums:"active,draft,inactive"`
	Have_form        bool               `json:"have_form,omitempty" example:"true"`

	Schedules []struct {
		Date        time.Time `json:"date" example:"2025-10-15T00:00:00Z"`
		TimeStart   time.Time `json:"time_start" example:"2025-10-15T09:00:00Z"`
		TimeEnd     time.Time `json:"time_end" example:"2025-10-15T12:00:00Z"`
		Location    string    `json:"location" example:"Innovation Building Room 301"`
		Description string    `json:"description" example:"Morning session"`
	} `json:"schedules"`
}

// Report DTO
type EventCreateResult struct {
	Event        models.Event           `json:"event"`
	Schedules    []models.EventSchedule `json:"schedules"`
	FormID       string                 `json:"form_id,omitempty"`
	OrganizerCnt int                    `json:"organizer_count"`
}

// Event Feed Detail each one
type EventFeed struct {
	EventID              string             `json:"event_id"`
	UserID			     string             `json:"user_id"`
	OrgPath              string             `json:"orgpath"`
	Topic                string             `json:"topic"`
	Description          string             `json:"description"`
	PictureURL           *string            `json:"picture_url,omitempty"`
	MaxParticipation     int                `json:"max_participation"`
	CurrentParticipation int                `json:"current_participation"`
	PostedAs             *models.PostedAs   `json:"posted_as,omitempty"`
	Visibility           *models.Visibility `json:"visibility,omitempty"`
	Status               string             `json:"status,omitempty"`
	Have_form            bool               `json:"have_form,omitempty"`
	CreatedAt		   	 *time.Time          `json:"created_at,omitempty"`	

	Schedules []models.EventSchedule `json:"schedules"`
}

// Event Detail
type EventDetail struct {
	EventID              string             `json:"event_id"`
	UserID 		   	     string             `json:"user_id"`
	OrgPath              string             `json:"orgpath"`
	Topic                string             `json:"topic"`
	Description          string             `json:"description"`
	PictureURL           *string            `json:"picture_url,omitempty"`
	MaxParticipation     int                `json:"max_participation"`
	CurrentParticipation int                `json:"current_participation"`
	PostedAs             *models.PostedAs   `json:"posted_as,omitempty"`
	Visibility           *models.Visibility `json:"visibility,omitempty"`
	Status               string             `json:"status,omitempty"`
	Have_form            bool               `json:"have_form,omitempty"`

	Schedules 			[]models.EventSchedule 	`json:"schedules"`
	UserParticipants    []UserParticipant       `json:"user_participants,omitempty"`		
}

type UserParticipant struct {
	UserID    string `json:"user_id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Status    string `json:"status"` //"participant/organizer"
}
