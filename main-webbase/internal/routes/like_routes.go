package routes

import (
    "context"
    "log"
    "time"

    "main-webbase/internal/controllers"

    "github.com/gofiber/fiber/v2"
    "go.mongodb.org/mongo-driver/v2/bson"
    "go.mongodb.org/mongo-driver/v2/mongo"
    "go.mongodb.org/mongo-driver/v2/mongo/options"
)

func ensureLikeIndexes(ctx context.Context, db *mongo.Database) error {
    likes := db.Collection("like")
    _, err := likes.Indexes().CreateMany(ctx, []mongo.IndexModel{
        {
            Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "post_id", Value: 1}},
            Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{
                "post_id": bson.M{"$exists": true},
            }),
        },
        {
            Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "comment_id", Value: 1}},
            Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{
                "comment_id": bson.M{"$exists": true},
            }),
        },
    })
    return err
}

func LikeRoutes(app *fiber.App, client *mongo.Client) {
    // Ensure unique toggle behavior by enforcing one-like-per-user per target
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    if err := ensureLikeIndexes(ctx, client.Database("unicom")); err != nil {
        log.Printf("[like] ensure indexes failed: %v", err)
    }

    post := app.Group("/likes")
    post.Post("/", controllers.LikeUnlikeHandler(client))
}
