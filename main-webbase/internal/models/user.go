package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type User struct {
	ID         bson.ObjectID `bson:"_id,omitempty" json:"_id,omitempty"`
	FirstName  string        `bson:"firstname" json:"firstname"`
	LastName   string        `bson:"lastname" json:"lastname"`
	ThaiPrefix string        `bson:"thaiprefix,omitempty" json:"thaiprefix,omitempty"`
	Gender     string        `bson:"gender,omitempty" json:"gender,omitempty"`
	TypePerson string        `bson:"type_person,omitempty" json:"type_person,omitempty"`
	StudentID  string        `bson:"student_id,omitempty" json:"student_id,omitempty"`
	AdvisorID  string        `bson:"advisor_id,omitempty" json:"advisor_id,omitempty"`
	ProfilePic string        `bson:"profile_pic,omitempty" json:"profile_pic,omitempty"` // ✅ add this

	// ADD เพิ่ม
	Disease      string    `bson:"disease,omitempty" json:"disease,omitempty"`
	Allergy      string    `bson:"allergy,omitempty" json:"allergy,omitempty"`
	Telephone    string    `bson:"telephone" json:"telephone"`
	Email        string    `bson:"email" json:"email"`
	PasswordHash string    `bson:"password_hash,omitempty" json:"-"`
	CreatedAt    time.Time `bson:"createdAt,omitempty" json:"createdAt,omitempty"`
	UpdatedAt    time.Time `bson:"updatedAt,omitempty" json:"updatedAt,omitempty"`
	// OTP          string    `bson:"otp"`
	// OTPExpiresAt time.Time `bson:"otp_expires_at"`
}

// User
// UserFirstname
// UserLastname
// Othercomponent...
// Membership 1 (cspk member) {
// Orgunit detail
// Position detail
// Policy detail
// }
// Membership 2 (comengineer student) {
// Orgunit detail
// Position detail
// Policy detail
// }
