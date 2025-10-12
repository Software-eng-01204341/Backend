package models

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Like struct {
	ID        bson.ObjectID `json:"id"         bson:"_id,omitempty"`
	UserID    bson.ObjectID `json:"userId"     bson:"user_id"`
	PostID    *bson.ObjectID `json:"postId"     bson:"post_id"`
	CommentID *bson.ObjectID `json:"commentId" bson:"comment_id"`
	CreatedAt time.Time     `json:"createdAt"  bson:"created_at"`
}