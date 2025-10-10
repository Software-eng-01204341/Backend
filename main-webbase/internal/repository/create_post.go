package repository

import (
	"context"
	"errors"
	"main-webbase/dto"
	"main-webbase/internal/models"
	"main-webbase/internal/utils"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// แปลง org_path → node_id (ObjectID) จาก collection org_units
func ResolveOrgNodeIDByPath(db *mongo.Database, orgPath string, ctx context.Context) (bson.ObjectID, error) {
	var doc struct {
		ID bson.ObjectID `bson:"_id"`
	}
	err := db.Collection("org_units").FindOne(ctx, bson.M{"org_path": orgPath, "status": "active"}).Decode(&doc)
	return doc.ID, err
}

// แปลง position_key → pos_id (ObjectID) จาก collection positions
func ResolvePositionIDByKey(db *mongo.Database, positionKey string, ctx context.Context) (bson.ObjectID, error) {
	var doc struct {
		ID bson.ObjectID `bson:"_id"`
	}
	err := db.Collection("positions").FindOne(ctx, bson.M{"key": positionKey, "status": "active"}).Decode(&doc)
	return doc.ID, err
}

func RebuildHashtags(db *mongo.Database, post models.Post, text string, ctx context.Context) error {
	hashtagsCol := db.Collection("hashtags")
	// ลบอันเก่า
	if _, err := hashtagsCol.DeleteMany(ctx, bson.M{
		"$or": []bson.M{
			{"post_id": post.ID},
			{"postId": post.ID},
		},
	}); err != nil {
		return err
	}

	// สร้างอันใหม่
	hashtags := utils.ExtractHashtags(text)
	if len(hashtags) == 0 {
		return nil
	}

	dateOnly := post.CreatedAt.Format("2006-01-02")
	docs := make([]interface{}, 0, len(hashtags))
	for _, tag := range hashtags {
		clean := strings.TrimSpace(strings.TrimPrefix(tag, "#"))
		if clean == "" {
			continue
		}
		docs = append(docs, models.PostHashtag{
			PostID: post.ID,
			Tag:    tag,
			Date:   dateOnly,
		})
	}
	if len(docs) == 0 {
		return nil
	}
	_, err := hashtagsCol.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false))
	return err
}

func ReplaceCategories(db *mongo.Database, postID bson.ObjectID, categoryIDs []string, ctx context.Context) error {
	col := db.Collection("post_categories")

	// 1) ลบของเก่า
	if _, err := col.DeleteMany(ctx, bson.M{"post_id": postID}); err != nil {
		return err
	}

	// 2) ไม่ส่งอะไรมาก็ไม่ต้องเพิ่ม
	if len(categoryIDs) == 0 {
		return nil
	}

	// 3) ใส่ของใหม่
	docs := make([]interface{}, 0, len(categoryIDs))
	for i, cidStr := range categoryIDs {
		cid, err := bson.ObjectIDFromHex(cidStr)
		if err != nil {
			return errors.New("invalid categoryId: " + cidStr)
		}
		docs = append(docs, models.PostCategory{
			PostID:     postID,
			CategoryID: cid,
			OrderIndex: i + 1,
		})
	}
	_, err := col.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false))
	return err
}

func ReplaceRoleVisibility(db *mongo.Database, postID bson.ObjectID, visibility dto.Visibility, ctx context.Context) error {
	col := db.Collection("post_role_visibility")

	// 1) ลบของเก่า
	if _, err := col.DeleteMany(ctx, bson.M{"post_id": postID}); err != nil {
		return err
	}

	// 2) public (audience ว่าง)
	if len(visibility.Audience) == 0 {
		_, err := col.InsertOne(ctx, models.PostRoleVisibility{
			PostID: postID,
			NodeID: nil,
		})
		return err
	}

	// เตรียม map org_path → node_id จากตาราง org_units
	nodeCol := db.Collection("org_units")
	docs := make([]interface{}, 0, len(visibility.Audience))

	for _, aud := range visibility.Audience {
		var nodeDoc struct {
			ID     bson.ObjectID `bson:"_id"`
			Path   string        `bson:"org_path"`
			Status string        `bson:"status"`
		}
		err := nodeCol.FindOne(ctx, bson.M{"org_path": aud, "status": "active"}).Decode(&nodeDoc)
		if err != nil {
			return errors.New("org_path not found: " + aud)
		}

		doc := models.PostRoleVisibility{
			PostID: postID,
			NodeID: &nodeDoc.ID, // pointer เพื่อเก็บ ObjectID
		}
		docs = append(docs, doc)
	}

	_, err := col.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false))
	return err
}

func FindUserInfo(col *mongo.Collection, userID bson.ObjectID, ctx context.Context) (user dto.UserInfoResponse, err error) {
    // Some databases may not store a user status; fall back gracefully.
    // 1) Try decode into UserInfoResponse without enforcing status
    if e := col.FindOne(ctx, bson.M{"_id": userID}).Decode(&user); e == nil {
        return user, nil
    } else {
        err = e
    }
    // 2) Fallback to models.User schema and map minimal fields
    var mu models.User
    if e := col.FindOne(ctx, bson.M{"_id": userID}).Decode(&mu); e == nil {
        user.ID = mu.ID
        user.FirstName = mu.FirstName
        user.LastName = mu.LastName
        user.Email = mu.Email
        if user.Status == "" {
            user.Status = "active"
        }
        return user, nil
    }
    return user, err
}

// GetIndividualPostDetail
func FindPostByID(col *mongo.Collection, id bson.ObjectID, ctx context.Context) (models.Post, error) {
	var p models.Post
	err := col.FindOne(ctx, bson.M{"_id": id}).Decode(&p)
	return p, err
}
func FindPositionName(col *mongo.Collection, id bson.ObjectID, ctx context.Context) (string, error) {
	var doc struct {
		Name string `bson:"key"`
	}
	err := col.FindOne(ctx, bson.M{"_id": id, "status": "active"}).Decode(&doc)
	if err != nil {
		return "", err
	}
	return doc.Name, nil
}
func FindOrgNode(col *mongo.Collection, id bson.ObjectID, ctx context.Context) (string, error) {
	var doc struct {
		OrgPath string `bson:"org_path"`
	}
	err := col.FindOne(ctx, bson.M{"_id": id, "status": "active"}).Decode(&doc)
	return doc.OrgPath, err
}

// ดึง visibility ของโพสต์จาก post_role_visibility -> แปลง role_id เป็น org_units.path
func FindVisibilityPaths(
	colPRV *mongo.Collection, // post_role_visibility
	colOrg *mongo.Collection, // org_units
	postID bson.ObjectID,
	ctx context.Context,
) (dto.Visibility, error) {

	// 1) หา role_id ทั้งหมดที่ผูกกับ post_id
	cur, err := colPRV.Find(ctx,
		bson.M{"post_id": postID},
		options.Find().SetProjection(bson.M{"role_id": 1}))
	if err != nil {
		return dto.Visibility{}, err
	}
	defer cur.Close(ctx)

	roleIDs := make([]bson.ObjectID, 0, 8)
	for cur.Next(ctx) {
		var row struct {
			RoleID bson.ObjectID `bson:"role_id"`
		}
		if err := cur.Decode(&row); err != nil {
			return dto.Visibility{}, err
		}
		roleIDs = append(roleIDs, row.RoleID)
	}
	if err := cur.Err(); err != nil {
		return dto.Visibility{}, err
	}

	// ถ้าไม่มี record → access=public, audience=[]
	if len(roleIDs) == 0 {
		return dto.Visibility{Access: "public", Audience: []string{}}, nil
	}

	// 2) มี record → access=private แล้ว resolve path จาก org_units
	//    (สมมติ role_id == _id ของ org_units; ถ้า schema จริงต่างออกไป ให้ปรับ join ตรงนี้)
	cur2, err := colOrg.Find(ctx,
		bson.M{"_id": bson.M{"$in": roleIDs}, "status": "active"},
		options.Find().SetProjection(bson.M{"org_path": 1}))
	if err != nil {
		return dto.Visibility{}, err
	}
	defer cur2.Close(ctx)

	// เก็บ path แบบ unique
	pathSet := make(map[string]struct{}, len(roleIDs))
	for cur2.Next(ctx) {
		var node struct {
			Path string `bson:"org_path"`
		}
		if err := cur2.Decode(&node); err != nil {
			return dto.Visibility{}, err
		}
		if node.Path != "" {
			pathSet[node.Path] = struct{}{}
		}
	}
	if err := cur2.Err(); err != nil {
		return dto.Visibility{}, err
	}

	audience := make([]string, 0, len(pathSet))
	for p := range pathSet {
		audience = append(audience, p)
	}

	return dto.Visibility{Access: "private", Audience: audience}, nil
}

// ดึง category_ids ของโพสต์จาก post_categories
func FindCategoryIDs(col *mongo.Collection, postID bson.ObjectID, ctx context.Context) ([]string, error) {
	cur, err := col.Find(ctx, bson.M{"post_id": postID}, options.Find().SetProjection(bson.M{"category_id": 1}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var ids []string
	for cur.Next(ctx) {
		var row struct {
			CategoryID bson.ObjectID `bson:"category_id"`
		}
		if err := cur.Decode(&row); err != nil {
			return nil, err
		}
		ids = append(ids, row.CategoryID.Hex())
	}
	return ids, cur.Err()
}

// DeletePostHandler
func DeletePost(db *mongo.Database, postID bson.ObjectID, ctx context.Context, userID bson.ObjectID, isRoot bool) (bool, error) {
	col := db.Collection("posts")

	// filter หาจาก _id
	filter := bson.M{"_id": postID, "status": "active"}

	if !isRoot {
		filter["user_id"] = userID
	}

	update := bson.M{
		"$set": bson.M{
			"status": "inactive",
		},
	}

	res, err := col.UpdateOne(ctx, filter, update)
	if err != nil {
		return false, err
	}
	if res.MatchedCount == 0 {
		// ไม่ตรงเงื่อนไข: ไม่ใช่เจ้าของ, ไม่ใช่ admin, หรือโพสต์ไม่ active/ไม่พบ
		return false, nil
	}
	return res.ModifiedCount > 0, nil
}

// UpdatePostCore: อัปเดตเอกสาร posts (owner-or-admin)
// - owner: แก้เนื้อหาได้ แต่ "ห้าม" เปลี่ยน status
// - admin: แก้ได้ทั้งหมด รวมถึง status
func UpdatePostCore(
	db *mongo.Database,
	postID, userID bson.ObjectID,
	isRoot bool,
	in dto.UpdatePostFullDTO,
	rolePathID, positionID bson.ObjectID,
	ctx context.Context,
) (*models.Post, error) {

	col := db.Collection("posts")

	filter := bson.M{"_id": postID, "status": "active"}
	if !isRoot {
		// เจ้าของเท่านั้นถ้าไม่ใช่แอดมิน
		filter["user_id"] = userID
	}
	newHashtags := utils.ExtractHashtags(in.PostText)
	// fmt.Printf("[UpdatePost] post=%s user=%s admin=%v\n", postID.Hex(), userID.Hex(), isRoot)
	// fmt.Printf("[UpdatePost] filter=%v\n", filter)
	set := bson.M{
		"post_text":   in.PostText,
		"picture_url": in.PictureUrl,
		"video_url":   in.VideoUrl,
		"node_id":     rolePathID, // map จาก org_path
		"position_id": positionID, // map จาก position_key
		"tags":        in.PostAs.Tag,
		"hashtag":     newHashtags,
		"updated_at":  time.Now().UTC(),
	}

	// อนุญาตให้ admin เปลี่ยน status ได้เท่านั้น
	if isRoot && in.Status != "" {
		set["status"] = in.Status
	}

	update := bson.M{"$set": set}
	after := options.FindOneAndUpdate().SetReturnDocument(options.After)

	var out models.Post
	if err := col.FindOneAndUpdate(ctx, filter, update, after).Decode(&out); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // ให้ service/handler แปลเป็น 403/404 เอง
		}
		return nil, err
	}
	return &out, nil
}
