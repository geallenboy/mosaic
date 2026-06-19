package auth

import (
	"context"
	"testing"
	"time"

	"github.com/mosaic-app/mosaic/backend/internal/repository"
)

func TestServiceRegisterHashesPasswordAndIssuesTokens(t *testing.T) {
	store := newFakeUserStore()
	service := NewService(store, Config{
		JWTSecret:          "test-secret-with-at-least-32-characters",
		AccessTokenExpiry:  time.Minute,
		RefreshTokenExpiry: time.Hour,
	})

	resp, err := service.Register(context.Background(), RegisterRequest{
		Email:       "User@Example.com",
		Password:    "password123",
		DisplayName: "User",
	})
	if err != nil {
		t.Fatalf("register returned error: %v", err)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatal("expected access and refresh tokens")
	}
	if store.created.PasswordHash == "password123" {
		t.Fatal("expected password to be hashed")
	}
	if resp.User.Email != "user@example.com" {
		t.Fatalf("expected normalized email, got %q", resp.User.Email)
	}
}

func TestServiceLoginRejectsWrongPassword(t *testing.T) {
	store := newFakeUserStore()
	service := NewService(store, Config{
		JWTSecret:          "test-secret-with-at-least-32-characters",
		AccessTokenExpiry:  time.Minute,
		RefreshTokenExpiry: time.Hour,
	})
	if _, err := service.Register(context.Background(), RegisterRequest{
		Email:       "user@example.com",
		Password:    "password123",
		DisplayName: "User",
	}); err != nil {
		t.Fatalf("register returned error: %v", err)
	}

	if _, err := service.Login(context.Background(), LoginRequest{
		Email:    "user@example.com",
		Password: "wrong-password",
	}); err == nil {
		t.Fatal("expected wrong password to be rejected")
	}
}

func TestServiceValidatesAccessAndRefreshTokens(t *testing.T) {
	store := newFakeUserStore()
	service := NewService(store, Config{
		JWTSecret:          "test-secret-with-at-least-32-characters",
		AccessTokenExpiry:  time.Minute,
		RefreshTokenExpiry: time.Hour,
	})
	resp, err := service.Register(context.Background(), RegisterRequest{
		Email:       "user@example.com",
		Password:    "password123",
		DisplayName: "User",
	})
	if err != nil {
		t.Fatalf("register returned error: %v", err)
	}

	claims, err := service.ValidateAccessToken(resp.AccessToken)
	if err != nil {
		t.Fatalf("validate access token returned error: %v", err)
	}
	if claims.UserID != resp.User.ID {
		t.Fatalf("expected user id %q, got %q", resp.User.ID, claims.UserID)
	}

	refreshed, err := service.Refresh(context.Background(), resp.RefreshToken)
	if err != nil {
		t.Fatalf("refresh returned error: %v", err)
	}
	if refreshed.AccessToken == "" || refreshed.RefreshToken == "" {
		t.Fatal("expected refreshed tokens")
	}
}

type fakeUserStore struct {
	users   map[string]*repository.User
	created repository.CreateUserParams
}

func newFakeUserStore() *fakeUserStore {
	return &fakeUserStore{users: make(map[string]*repository.User)}
}

func (s *fakeUserStore) CreateUser(_ context.Context, params repository.CreateUserParams) (*repository.User, error) {
	s.created = params
	user := &repository.User{
		ID:           "user-1",
		Email:        params.Email,
		DisplayName:  params.DisplayName,
		PasswordHash: params.PasswordHash,
		Role:         "user",
		Status:       "active",
	}
	s.users[user.Email] = user
	return user, nil
}

func (s *fakeUserStore) GetUserByEmail(_ context.Context, email string) (*repository.User, error) {
	user, ok := s.users[email]
	if !ok {
		return nil, repository.ErrUserNotFound
	}
	return user, nil
}

func (s *fakeUserStore) GetUserByID(_ context.Context, id string) (*repository.User, error) {
	for _, user := range s.users {
		if user.ID == id {
			return user, nil
		}
	}
	return nil, repository.ErrUserNotFound
}
