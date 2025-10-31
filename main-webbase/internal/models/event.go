package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Event struct {
	ID               bson.ObjectID `bson:"_id,omitempty" json:"id"`
	NodeID           bson.ObjectID `bson:"node_id" json:"node_id"`
	Topic            string        `bson:"topic" json:"topic"`
	Description      string        `bson:"description" json:"description"`
	PictureURL       *string       `bson:"picture_url,omitempty" json:"picture_url,omitempty"`
	MaxParticipation int           `bson:"max_participation" json:"max_participation"`

	PostedAs     *PostedAs   `json:"posted_as,omitempty"`
	Visibility   *Visibility `json:"visibility,omitempty"`
	OrgOfContent string      `bson:"org_of_content,omitempty" json:"org_of_content,omitempty"`
	Status       string      `bson:"status,omitempty" json:"status,omitempty"` // active, pending, draft, inactive
	Have_form    bool        `bson:"have_form" json:"have_form"`

	CreatedAt *time.Time `bson:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt *time.Time `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
}

type EventSchedule struct {
	ID          bson.ObjectID `bson:"_id,omitempty" json:"id"`
	EventID     bson.ObjectID `bson:"event_id" json:"event_id"`
	Date        time.Time     `bson:"date" json:"date"`
	Time_start  time.Time     `bson:"time_start" json:"time_start"`
	Time_end    time.Time     `bson:"time_end" json:"time_end"`
	Location    *string       `bson:"location,omitempty" json:"location,omitempty"`
	Description *string       `bson:"description,omitempty" json:"description,omitempty"`
}
