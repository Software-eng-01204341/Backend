package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"main-webbase/database"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	mongo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ------- cursor helpers (no primitive) -------
type postCursor struct {
	T  time.Time `json:"t"`
	ID string    `json:"id"` // hex of bson.ObjectID
}

func decodeCursor(s string) (time.Time, bson.ObjectID, error) {
	if s == "" {
		return time.Time{}, bson.ObjectID{}, fmt.Errorf("empty cursor")
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return time.Time{}, bson.ObjectID{}, err
	}
	var pc postCursor
	if err := json.Unmarshal(raw, &pc); err != nil {
		return time.Time{}, bson.ObjectID{}, err
	}
	oid, err := bson.ObjectIDFromHex(pc.ID)
	if err != nil {
		return time.Time{}, bson.ObjectID{}, err
	}
	return pc.T.UTC(), oid, nil
}

func encodePostCursor(t time.Time, id bson.ObjectID) string {
	pc := postCursor{T: t.UTC(), ID: id.Hex()}
	b, _ := json.Marshal(pc)
	return base64.RawURLEncoding.EncodeToString(b)
}

// ------- main repo -------

// visibility:

func ListAllPostsVisibleToViewer(
	ctx context.Context,
	client *mongo.Client,
	cursorStr string,
	limit int64,
	allowedRoleIDs []bson.ObjectID, // สิทธิ์ของผู้ดู (node/role IDs ที่เข้าถึงได้)
) (items []bson.M, next *string, err error) {

	postsColl := database.DB.Collection("posts")

	// cursor match (created_at, _id)
	var cursorMatch bson.D
	if cursorStr != "" {
		t, oid, e := decodeCursor(cursorStr) // ⬅️ no primitive
		if e != nil {
			return nil, nil, e
		}
		cursorMatch = bson.D{{
			Key: "$or",
			Value: bson.A{
				bson.D{{Key: "created_at", Value: bson.D{{Key: "$lt", Value: t}}}},
				bson.D{{Key: "created_at", Value: t}, {Key: "_id", Value: bson.D{{Key: "$lt", Value: oid}}}},
			},
		}}
	}

	// 3) pipeline หลัก
	pipe := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{
			{Key: "status", Value: "active"},
		}}},
	}
	if len(cursorMatch) > 0 {
		pipe = append(pipe, bson.D{{Key: "$match", Value: cursorMatch}})
	}

	pipe = append(pipe,
		// เผื่อ created_at เป็น string -> toDate
		bson.D{{Key: "$addFields", Value: bson.D{
			{Key: "created_at", Value: bson.D{{Key: "$toDate", Value: "$created_at"}}},
		}}},
		// join prv
		bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "from", Value: "post_role_visibility"},
			{Key: "localField", Value: "_id"},
			{Key: "foreignField", Value: "post_id"},
			{Key: "as", Value: "prv"},
		}}},
	)

	pipe = append(pipe, bson.D{{Key: "$match", Value: bson.D{
		{Key: "$or", Value: bson.A{
			bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq",
				Value: bson.A{bson.D{{Key: "$size", Value: "$prv"}}, 0}}}}},
			bson.D{{Key: "$expr", Value: bson.D{{Key: "$gt",
				Value: bson.A{
					bson.D{{Key: "$size", Value: bson.D{{Key: "$ifNull", Value: bson.A{
						bson.D{{Key: "$setIntersection", Value: bson.A{"$prv.node_id", allowedRoleIDs}}},
						bson.A{},
					}}}}},
					0,
				}}},
			}},
		}},
	}}})

	// คำนวณ field เสริม
	pipe = append(pipe,
		bson.D{{Key: "$addFields", Value: bson.D{
			{Key: "visibility", Value: bson.D{
				{Key: "$cond", Value: bson.A{
					bson.D{{Key: "$gt", Value: bson.A{bson.D{{Key: "$size", Value: "$prv"}}, 0}}},
					"private", "public",
				}},
			}},
			{Key: "matched_node_ids", Value: bson.D{
				{Key: "$setIntersection", Value: bson.A{"$prv.node_id", allowedRoleIDs}},
			}},
		}}},
		bson.D{{Key: "$addFields", Value: bson.D{
			{Key: "like_count", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$like_count", 0}}}},
			{Key: "comment_count", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$comment_count", 0}}}},
		}}},
		bson.D{{Key: "$project", Value: bson.D{{Key: "prv", Value: 0}}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "created_at", Value: -1}, {Key: "_id", Value: -1}}}},
		bson.D{{Key: "$limit", Value: limit + 1}},
	)

	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cur, e := postsColl.Aggregate(cctx, pipe, options.Aggregate())
	if e != nil {
		return nil, nil, e
	}
	defer cur.Close(cctx)

	var all []bson.M
	if e := cur.All(cctx, &all); e != nil {
		return nil, nil, e
	}

	// 4) next cursor
	if int64(len(all)) > limit {
		items = all[:limit]
		last := items[len(items)-1]

		// created_at
		var tm time.Time
		switch v := last["created_at"].(type) {
		case time.Time:
			tm = v.UTC()
		case bson.DateTime:
			tm = time.UnixMilli(int64(v)).UTC()
		case string:
			if t2, perr := time.Parse(time.RFC3339Nano, v); perr == nil {
				tm = t2.UTC()
			} else if t2, perr := time.Parse(time.RFC3339, v); perr == nil {
				tm = t2.UTC()
			} else {
				return nil, nil, fmt.Errorf("invalid created_at string: %q", v)
			}
		default:
			return nil, nil, fmt.Errorf("unknown created_at type: %T", v)
		}

		// _id → bson.ObjectID (รองรับได้ทั้ง string hex และ bson.ObjectID)
		var lastID bson.ObjectID
		switch v := last["_id"].(type) {
		case bson.ObjectID:
			lastID = v
		case string:
			oid, perr := bson.ObjectIDFromHex(v)
			if perr != nil {
				return nil, nil, fmt.Errorf("invalid _id hex in page item: %q", v)
			}
			lastID = oid
		default:
			return nil, nil, fmt.Errorf("unknown _id type: %T", v)
		}

		s := encodePostCursor(tm, lastID)
		next = &s
	} else {
		items = all
		next = nil
	}

	return
}
