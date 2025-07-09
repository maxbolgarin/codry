package main

import (
	"context"
	"fmt"
	"net/http"
)

// User represents a user in the system
type User struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Active bool   `json:"active"`
}

// UserRepository interface for user data access
type UserRepository interface {
	GetUser(ctx context.Context, id int) (*User, error)
	CreateUser(ctx context.Context, user *User) error
	UpdateUser(ctx context.Context, user *User) error
	DeleteUser(ctx context.Context, id int) error
}

// userService handles user business logic
type userService struct {
	repo UserRepository
	log  Logger
}

// Logger interface for logging
type Logger interface {
	Info(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

const (
	MaxUsers    = 1000
	DefaultPort = 8080
)

var (
	globalCounter int
	isInitialized bool = false
)

// NewUserService creates a new user service
func NewUserService(repo UserRepository, logger Logger) *userService {
	return &userService{
		repo: repo,
		log:  logger,
	}
}

// GetUser retrieves a user by ID
func (s *userService) GetUser(ctx context.Context, id int) (*User, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid user ID: %d", id)
	}

	user, err := s.repo.GetUser(ctx, id)
	if err != nil {
		s.log.Error("failed to get user", "id", id, "error", err)
		return nil, err
	}

	s.log.Info("retrieved user", "id", id, "name", user.Name)
	return user, nil
}

// CreateUser creates a new user
func (s *userService) CreateUser(ctx context.Context, user *User) error {
	if user == nil {
		return fmt.Errorf("user cannot be nil")
	}

	if err := validateUser(user); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	return s.repo.CreateUser(ctx, user)
}

// UpdateUser updates an existing user
func (s *userService) UpdateUser(ctx context.Context, user *User) error {
	existing, err := s.GetUser(ctx, user.ID)
	if err != nil {
		return err
	}

	if existing == nil {
		return fmt.Errorf("user not found")
	}

	return s.repo.UpdateUser(ctx, user)
}

// validateUser validates user data
func validateUser(user *User) error {
	if user.Name == "" {
		return fmt.Errorf("name is required")
	}
	if user.Email == "" {
		return fmt.Errorf("email is required")
	}
	return nil
}

// HandleUserRequest handles HTTP requests for users
func HandleUserRequest(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleGetUser(w, r)
	case http.MethodPost:
		handleCreateUser(w, r)
	case http.MethodPut:
		handleUpdateUser(w, r)
	case http.MethodDelete:
		handleDeleteUser(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetUser handles GET requests
func handleGetUser(w http.ResponseWriter, r *http.Request) {
	// Implementation here
	fmt.Fprintf(w, "Get user")
}

// handleCreateUser handles POST requests
func handleCreateUser(w http.ResponseWriter, r *http.Request) {
	// Implementation here
	fmt.Fprintf(w, "Create user")
}

// handleUpdateUser handles PUT requests
func handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	// Implementation here
	fmt.Fprintf(w, "Update user")
}

// handleDeleteUser handles DELETE requests
func handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	// Implementation here
	fmt.Fprintf(w, "Delete user")
}

// init initializes the package
func init() {
	globalCounter = 0
	isInitialized = true
}

// main is the entry point
func main() {
	fmt.Println("Starting user service...")

	// Anonymous function
	processData := func(data []string) []string {
		result := make([]string, len(data))
		for i, item := range data {
			result[i] = fmt.Sprintf("processed: %s", item)
		}
		return result
	}

	data := []string{"item1", "item2", "item3"}
	processed := processData(data)
	fmt.Printf("Processed data: %v\n", processed)
}
