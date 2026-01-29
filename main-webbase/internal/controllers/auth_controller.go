package controllers

import (
	"context"
	"crypto/rand"
	"fmt"
	"main-webbase/database"
	"main-webbase/internal/models"
	"net/smtp"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// GenerateOTP generates a numeric OTP of given length
func GenerateOTP(length int) (string, error) {
	const digits = "0123456789"
	otp := make([]byte, length)
	_, err := rand.Read(otp)
	if err != nil {
		return "", err
	}
	for i := 0; i < length; i++ {
		otp[i] = digits[otp[i]%10]
	}
	return string(otp), nil
}

// SendOTPEmail sends an OTP email to the user
func SendOTPEmail(toEmail, otp string) error {
	smtpHost := "smtp.zoho.com"
	smtpPort := "587"
	fromEmail := "otp@kucom.art" // replace with your domain email
	password := "tcq%ecM1"       // use environment variable in production

	subject := "Your Registration OTP"
	body := fmt.Sprintf("Hello,\n\nYour OTP code is: %s\nThis code will expire in 5 minutes.\n\n", otp)
	message := fmt.Sprintf("From: %s\nTo: %s\nSubject: %s\n\n%s", fromEmail, toEmail, subject, body)

	auth := smtp.PlainAuth("", fromEmail, password, smtpHost)
	return smtp.SendMail(smtpHost+":"+smtpPort, auth, fromEmail, []string{toEmail}, []byte(message))
}

// Register godoc
// @Summary Register a new user
// @Description Create a new user account with hashed password and send OTP
// @Tags auth
// @Accept json
// @Produce json
// @Param registerRequest body models.RegisterRequest true "Register Request"
// @Success 201 {object} map[string]interface{} "User registered successfully, OTP sent"
// @Failure 400 {object} map[string]interface{} "Invalid request body or email already exists"
// @Failure 500 {object} map[string]interface{} "Failed to create user"
// @Router /register [post]
func Register(c *fiber.Ctx) error {
	collection := database.DB.Collection("users")

	var registerRequest models.RegisterRequest
	if err := c.BodyParser(&registerRequest); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if !strings.HasSuffix(registerRequest.Email, "@ku.th") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Email must be a @ku.th address"})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if user exists
	var existUser models.User
	err := collection.FindOne(ctx, bson.M{"email": registerRequest.Email}).Decode(&existUser)
	if err == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Email already exists"})
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(registerRequest.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to hash password"})
	}

	// Generate OTP
	// otp, err := GenerateOTP(6)
	// if err != nil {
	// 	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate OTP"})
	// }

	// Send OTP email
	// if err := SendOTPEmail(registerRequest.Email, otp); err != nil {
	// 	log.Println("Failed to send OTP email:", err)
	// 	// Optional: continue registration even if email fails
	// }

	// Add to DB
	user := models.User{
		ID:           bson.NewObjectID(),
		FirstName:    registerRequest.FirstName,
		LastName:     registerRequest.LastName,
		ThaiPrefix:   registerRequest.ThaiPrefix,
		Gender:       registerRequest.Gender,
		TypePerson:   registerRequest.TypePerson,
		StudentID:    registerRequest.StudentID,
		AdvisorID:    registerRequest.AdvisorID,
		Email:        registerRequest.Email,
		PasswordHash: string(hashedPassword),
		Disease:      registerRequest.Disease,
		Allergy:      registerRequest.Allergy,
		Telephone:    registerRequest.Telephone,
		// OTP:          otp,                             // store OTP in DB
		// OTPExpiresAt: time.Now().Add(5 * time.Minute), // expires in 5 minutes
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	_, err = collection.InsertOne(ctx, user)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create user"})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"message": "User registered successfully. OTP sent to email.",
		"user_id": user.ID.Hex(),
	})
}

// Login godoc
// @Summary Login user
// @Description Authenticate user and return JWT token
// @Tags auth
// @Accept json
// @Produce json
// @Param loginRequest body models.LoginRequest true "Login Request"
// @Success 200 {object} map[string]interface{} "User and access token"
// @Failure 400 {object} map[string]interface{} "Invalid request body"
// @Failure 401 {object} map[string]interface{} "Invalid credentials"
// @Failure 500 {object} map[string]interface{} "Database or token error"
// @Router /login [post]
func Login(c *fiber.Ctx) error {
	var loginRequest models.LoginRequest
	if err := c.BodyParser(&loginRequest); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	if !strings.HasSuffix(loginRequest.Email, "@ku.th") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Email must be a @ku.th address"})
	}

	// Find the user in the database by their username
	collection := database.DB.Collection("users")

	var user models.User
	filter := bson.M{"email": loginRequest.Email}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := collection.FindOne(ctx, filter).Decode(&user); err != nil {
		if err == mongo.ErrNoDocuments {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid Email credentials"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database query failed"})
	}

	// Compare password and hash
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(loginRequest.Password)); err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid Password"})
	}

	// Create JWT Claims
	claims := jwt.MapClaims{
		"uid": user.ID.Hex(),
		"sub": user.ID.Hex(), // ทำไว้ 2 ชั้น เป็นมาตรฐานเอาไว้ใช้ใน Middleware ด้วย
		"exp": time.Now().Add(time.Hour * 72).Unix(),
	}

	// Create token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign token with a secret key (use an environment variable!)
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Missing JWT_SECRET"})
	}

	t, err := token.SignedString([]byte(secret))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Could not sign token"})
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"user":        user,
		"accessToken": t,
	})
}
