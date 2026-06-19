package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mosaic-app/mosaic/backend/internal/auth"
)

func TestAuthHandlerRegisterReturnsTokens(t *testing.T) {
	service := &fakeAuthService{
		response: &auth.Response{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
			ExpiresIn:    900,
			User: auth.UserInfo{
				ID:          "user-1",
				Email:       "user@example.com",
				DisplayName: "User",
				Role:        "user",
			},
		},
	}
	handler := NewAuthHandler(service)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{
		"email":"user@example.com",
		"password":"password123",
		"display_name":"User"
	}`))
	rec := httptest.NewRecorder()

	handler.Register(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}
	var body struct {
		Data struct {
			AccessToken string `json:"access_token"`
			User        struct {
				Email string `json:"email"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.AccessToken != "access-token" || body.Data.User.Email != "user@example.com" {
		t.Fatalf("unexpected response: %#v", body.Data)
	}
}

func TestAuthMiddlewareRejectsMissingBearerToken(t *testing.T) {
	service := &fakeAuthService{}
	middleware := AuthMiddleware(service)
	nextCalled := false
	next := middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		nextCalled = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/start", nil)
	next.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
	if nextCalled {
		t.Fatal("expected middleware to stop request")
	}
}

func TestAuthMiddlewareAddsClaimsToContext(t *testing.T) {
	service := &fakeAuthService{
		claims: &auth.Claims{
			UserID: "user-1",
			Email:  "user@example.com",
			Role:   "user",
		},
	}
	middleware := AuthMiddleware(service)
	next := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := CurrentUser(r.Context())
		if !ok || user.UserID != "user-1" {
			t.Fatalf("expected current user in context, got %#v ok=%v", user, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/start", nil)
	req.Header.Set("Authorization", "Bearer access-token")
	next.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
}

type fakeAuthService struct {
	response *auth.Response
	claims   *auth.Claims
}

func (s *fakeAuthService) Register(context.Context, auth.RegisterRequest) (*auth.Response, error) {
	return s.response, nil
}

func (s *fakeAuthService) Login(context.Context, auth.LoginRequest) (*auth.Response, error) {
	return s.response, nil
}

func (s *fakeAuthService) Refresh(context.Context, string) (*auth.Response, error) {
	return s.response, nil
}

func (s *fakeAuthService) ValidateAccessToken(string) (*auth.Claims, error) {
	if s.claims == nil {
		return nil, auth.ErrInvalidToken
	}
	return s.claims, nil
}
