package services

import (
	"context"
	"errors"
	"main-webbase/dto"
	"main-webbase/internal/models"
	"main-webbase/internal/repository"

	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// like หรือ unlike (toggle)
func Like(ctx context.Context, client *mongo.Client, body dto.LikeRequestDTO, userID bson.ObjectID) (int, any) {
	if err := validateLikeRequest(body); err != nil {
		return fiberStatusBadRequest(), dto.ErrorResponse{Error: err.Error()}
	}
	targetID, err := parseObjectIDs(body)
	if err != nil {
		return fiberStatusBadRequest(), dto.ErrorResponse{Error: err.Error()}
	}

	db := client.Database("unicom")
	likesCol := db.Collection("like")
	updateCol, likeDoc, targetFilter := buildLikeDocAndTarget(db, body, userID, targetID, time.Now().UTC())

	// 1) พยายาม insert (จะ fail เป็น dup ถ้าเคยไลก์แล้ว เพราะมี unique index)
	dup, err := repository.InsertLike(ctx, likesCol, likeDoc)
	if err != nil { // errors นอกเหนือจาก duplicate
		return fiberStatusInternalError(), dto.ErrorResponse{Error: err.Error()}
	}

	likedNow := false
	var likeCount int64
	if dup {
		// 2) เคยไลก์แล้ว → toggle เป็น unlike: ลบ doc + decrement counter
		if err := repository.DeleteLike(ctx, likesCol, userID, targetID, body.TargetType); err != nil {
			return fiberStatusInternalError(), dto.ErrorResponse{Error: err.Error()}
		}
		repository.DecLikeCount(ctx, updateCol, targetFilter)
		likedNow = false
	} else {
		// 2) ยังไม่เคย → like: เพิ่ม counter
		repository.IncLikeCount(ctx, updateCol, targetFilter)
		likedNow = true
	}

	likeCount, _ = repository.CountLikes(ctx, likesCol, targetID, body.TargetType)

	return fiberStatusOK(), bson.M{
		"Status":       ternary(likedNow, "liked", "unliked"),
		"target_id":   body.TargetID,
		"target_type": body.TargetType,
		"liked":       likedNow,  // เปลี่ยนชื่อ field ให้ FE ใช้ง่าย
		"likeCount":   likeCount, // ส่งยอดล่าสุดกลับไปทุกครั้ง
	}
}

func ternary[T any](cond bool, a, b T) T {
	if cond {
		return a
	}
	return b
}

// helper

func validateLikeRequest(body dto.LikeRequestDTO) error {
	if body.TargetType == "" || body.TargetID == "" {
		return errors.New("targetType, targetId are required")
	}
	if body.TargetType != "post" && body.TargetType != "comment" {
		return errors.New("targetType must be 'post' or 'comment'")
	}
	return nil
}

// แปลงเป็น ObjectID พร้อมตรวจสอบความถูกต้อง
func parseObjectIDs(body dto.LikeRequestDTO) (targetID bson.ObjectID, err error) {
	tid, terr := bson.ObjectIDFromHex(body.TargetID) // เดิม
	if terr != nil {
		return bson.ObjectID{}, errors.New("invalid targetId")
	}
	return tid, nil
}

// สร้าง likeDoc, กำหนด target collection(collection ที่เราจะ insert -> post/comment), ทำ filter เอาไปใช้ใน update (inc +1 like_count)
// คืนค่า: collection เป้าหมาย + likeDoc + filter เดิม
func buildLikeDocAndTarget(db *mongo.Database, body dto.LikeRequestDTO, userID bson.ObjectID, targetID bson.ObjectID, now time.Time) (*mongo.Collection, models.Like, bson.M) {
	var updateCol *mongo.Collection
	var likeDoc models.Like

	if body.TargetType == "post" {
		updateCol = db.Collection("posts")
		likeDoc = models.Like{
			UserID:    userID, // ใช้ชนิดเดิมของคุณ
			PostID:    &targetID,
			CommentID: nil,
			CreatedAt: now,
		}
	} else { // "comment"
		updateCol = db.Collection("comments")
		likeDoc = models.Like{
			UserID:    userID,
			PostID:    nil,
			CommentID: &targetID,
			CreatedAt: now,
		}
	}
	targetFilter := bson.M{"_id": targetID}
	return updateCol, likeDoc, targetFilter
}

// mapping สถานะ HTTP (แยกไว้ให้ชัด ไม่แก้ตัวเลขเดิม)
func fiberStatusOK() int            { return 200 }
func fiberStatusBadRequest() int    { return 400 }
func fiberStatusInternalError() int { return 500 }