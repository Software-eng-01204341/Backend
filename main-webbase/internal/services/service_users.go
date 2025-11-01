package services

import (
    "context"
    "time"

    "go.mongodb.org/mongo-driver/v2/bson"
    "go.mongodb.org/mongo-driver/v2/mongo"
    "go.mongodb.org/mongo-driver/v2/mongo/options"

    "main-webbase/database"
    "main-webbase/dto"
    repo "main-webbase/internal/repository"
)

func GetUserProfile(ctx context.Context, userID string) (*dto.UserProfileDTO, error) {
    users, err := repo.FindUserBy(ctx, "_id", userID)
    if err != nil {
        return nil, err
    }

    user := users[0]

    memberships, err := repo.GetUserMemberships(ctx, user.ID.Hex())
    if err != nil {
        return nil, err
    }

    var membershipDetails []dto.MembershipProfileDTO
    for _, m := range memberships {
        org, err := repo.FindByOrgPath(ctx, m.OrgPath)
        if err != nil {
            return nil, err
        }
        pos, err := repo.FindPositionByKeyandPath(ctx, m.PositionKey, m.OrgPath)
        if err != nil {
            return nil, err
        }
        policies, err := repo.FindPolicyByKeyandPath(ctx, m.PositionKey, m.OrgPath)
        if err != nil {
            return nil, err
        }

        membershipDetails = append(membershipDetails, dto.MembershipProfileDTO{
            MembershipName: pos.Display["en"],
            OrgUnit:        *org,
            Position:       *pos,
            Policies:       *policies,
        })
    }

    userprofile := &dto.UserProfileDTO{
        ID:          user.ID.Hex(),
        FirstName:   user.FirstName,
        LastName:    user.LastName,
        Email:       user.Email,
        ThaiPrefix:  user.ThaiPrefix,
        Gender:      user.Gender,
        TypePerson:  user.TypePerson,
        StudentID:   user.StudentID,
        AdvisorID:   user.AdvisorID,
        ProfilePic:  user.ProfilePic,
        Memberships: membershipDetails,
    }

    return userprofile, nil
}

// DeleteUserCascade removes user-related data so it no longer affects frontend, then deletes the user.
// Steps:
// - Adjust like_count on posts/comments for likes by this user, then delete those likes
// - Adjust comment_count on posts for comments by this user, delete likes on those comments, then delete those comments
// - Soft-hide posts authored by this user (status=inactive)
// - Deactivate memberships
// - Remove event participation and responses/answers, remove QA by this user
// - Inactivate events owned by this user if events have user_id
// - Finally delete the user document
func DeleteUserCascade(ctx context.Context, userIDHex string) error {
    oid, err := bson.ObjectIDFromHex(userIDHex)
    if err != nil {
        return err
    }

    db := database.DB

    // ===== Likes by user: decrement counts then delete =====
    likeCol := db.Collection("like")
    // posts.like_count
    cur, err := likeCol.Aggregate(ctx, mongo.Pipeline{
        { {Key: "$match", Value: bson.M{"user_id": oid, "post_id": bson.M{"$exists": true}} } },
        { {Key: "$group", Value: bson.M{"_id": "$post_id", "cnt": bson.M{"$sum": 1}} } },
    })
    if err == nil {
        var rows []struct{ ID bson.ObjectID `bson:"_id"`; Cnt int `bson:"cnt"` }
        if err := cur.All(ctx, &rows); err == nil {
            for _, r := range rows {
                _, _ = db.Collection("posts").UpdateOne(ctx, bson.M{"_id": r.ID}, bson.M{"$inc": bson.M{"like_count": -r.Cnt}})
            }
        }
        cur.Close(ctx)
    }
    // comments.like_count
    cur2, err := likeCol.Aggregate(ctx, mongo.Pipeline{
        { {Key: "$match", Value: bson.M{"user_id": oid, "comment_id": bson.M{"$exists": true}} } },
        { {Key: "$group", Value: bson.M{"_id": "$comment_id", "cnt": bson.M{"$sum": 1}} } },
    })
    if err == nil {
        var rows []struct{ ID bson.ObjectID `bson:"_id"`; Cnt int `bson:"cnt"` }
        if err := cur2.All(ctx, &rows); err == nil {
            for _, r := range rows {
                _, _ = db.Collection("comments").UpdateOne(ctx, bson.M{"_id": r.ID}, bson.M{"$inc": bson.M{"like_count": -r.Cnt}})
            }
        }
        cur2.Close(ctx)
    }
    // delete likes by user
    _, _ = likeCol.DeleteMany(ctx, bson.M{"user_id": oid})

    // ===== Comments by user: decrement post.comment_count, delete likes on those comments, delete comments =====
    commentsCol := db.Collection("comments")
    cur3, err := commentsCol.Aggregate(ctx, mongo.Pipeline{
        { {Key: "$match", Value: bson.M{"user_id": oid}} },
        { {Key: "$group", Value: bson.M{"_id": "$post_id", "cnt": bson.M{"$sum": 1}} } },
    })
    if err == nil {
        var rows []struct{ ID bson.ObjectID `bson:"_id"`; Cnt int `bson:"cnt"` }
        if err := cur3.All(ctx, &rows); err == nil {
            for _, r := range rows {
                _, _ = db.Collection("posts").UpdateOne(ctx, bson.M{"_id": r.ID}, bson.M{"$inc": bson.M{"comment_count": -r.Cnt}})
            }
        }
        cur3.Close(ctx)
    }
    // collect comment IDs to delete likes on them
    var cids []bson.ObjectID
    cur4, err := commentsCol.Find(ctx, bson.M{"user_id": oid}, options.Find().SetProjection(bson.M{"_id": 1}))
    if err == nil {
        for cur4.Next(ctx) {
            var row struct{ ID bson.ObjectID `bson:"_id"` }
            if err := cur4.Decode(&row); err == nil {
                cids = append(cids, row.ID)
            }
        }
        cur4.Close(ctx)
    }
    if len(cids) > 0 {
        _, _ = likeCol.DeleteMany(ctx, bson.M{"comment_id": bson.M{"$in": cids}})
    }
    _, _ = commentsCol.DeleteMany(ctx, bson.M{"user_id": oid})

    // ===== Soft-hide posts authored by this user =====
    _, _ = db.Collection("posts").UpdateMany(ctx, bson.M{"user_id": oid}, bson.M{"$set": bson.M{"status": "inactive"}})

    // ===== Deactivate memberships =====
    now := time.Now().UTC()
    _, _ = db.Collection("memberships").UpdateMany(ctx, bson.M{"user_id": oid}, bson.M{"$set": bson.M{"active": false, "ended_at": &now}})

    // ===== Event participation / responses / answers =====
    _, _ = db.Collection("event_participant").DeleteMany(ctx, bson.M{"user_id": oid})
    // responses and answers by this user
    var respIDs []bson.ObjectID
    respCur, err := db.Collection("event_response").Find(ctx, bson.M{"user_id": oid}, options.Find().SetProjection(bson.M{"_id": 1}))
    if err == nil {
        for respCur.Next(ctx) {
            var row struct{ ID bson.ObjectID `bson:"_id"` }
            if err := respCur.Decode(&row); err == nil {
                respIDs = append(respIDs, row.ID)
            }
        }
        respCur.Close(ctx)
    }
    if len(respIDs) > 0 {
        _, _ = db.Collection("event_form_answer").DeleteMany(ctx, bson.M{"response_id": bson.M{"$in": respIDs}})
        _, _ = db.Collection("event_response").DeleteMany(ctx, bson.M{"_id": bson.M{"$in": respIDs}})
    } else {
        _, _ = db.Collection("event_response").DeleteMany(ctx, bson.M{"user_id": oid})
    }
    // QA by this user
    _, _ = db.Collection("event_qa").DeleteMany(ctx, bson.M{"$or": []bson.M{{"questioner_id": oid}, {"answerer_id": oid}}})

    // ===== Inactivate events owned by this user, if such field exists =====
    _, _ = db.Collection("events").UpdateMany(ctx, bson.M{"user_id": oid}, bson.M{"$set": bson.M{"status": "inactive", "updated_at": now}})

    // ===== Notifications best-effort =====
    _, _ = db.Collection("notifications").DeleteMany(ctx, bson.M{"user_id": oid})
    _, _ = db.Collection("notification_queue").DeleteMany(ctx, bson.M{"user_id": oid})

    // ===== Finally delete user document =====
    _, _ = db.Collection("users").DeleteOne(ctx, bson.M{"_id": oid})
    return nil
}
