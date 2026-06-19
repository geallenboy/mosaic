package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/mosaic-app/mosaic/backend/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidToken       = errors.New("invalid token")
	ErrEmailExists        = errors.New("email already exists")
)

type Config struct {
	JWTSecret          string
	AccessTokenExpiry  time.Duration
	RefreshTokenExpiry time.Duration
}

type UserStore interface {
	CreateUser(ctx context.Context, params repository.CreateUserParams) (*repository.User, error)
	GetUserByEmail(ctx context.Context, email string) (*repository.User, error)
	GetUserByID(ctx context.Context, id string) (*repository.User, error)
}

type Service struct {
	store  UserStore
	config Config
}

type RegisterRequest struct {
	Email       string
	Password    string
	DisplayName string
}

type LoginRequest struct {
	Email    string
	Password string
}

type Response struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
	User         UserInfo
}

type UserInfo struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	DisplayName string  `json:"display_name"`
	Role        string  `json:"role"`
	AvatarURL   *string `json:"avatar_url,omitempty"`
}

type Claims struct {
	UserID    string `json:"uid"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	TokenType string `json:"typ"`
	jwt.RegisteredClaims
}

func NewService(store UserStore, config Config) *Service {
	return &Service{store: store, config: config}
}

func (s *Service) Register(ctx context.Context, req RegisterRequest) (*Response, error) {
	email := normalizeEmail(req.Email)
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.store.CreateUser(ctx, repository.CreateUserParams{
		Email:        email,
		DisplayName:  strings.TrimSpace(req.DisplayName),
		PasswordHash: string(hash),
	})
	if errors.Is(err, repository.ErrUserConflict) {
		return nil, ErrEmailExists
	}
	if err != nil {
		return nil, err
	}
	return s.buildResponse(user)
}

func (s *Service) Login(ctx context.Context, req LoginRequest) (*Response, error) {
	user, err := s.store.GetUserByEmail(ctx, normalizeEmail(req.Email))
	if errors.Is(err, repository.ErrUserNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if user.Status != "active" {
		return nil, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	return s.buildResponse(user)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (*Response, error) {
	claims, err := s.parseToken(refreshToken, "refresh")
	if err != nil {
		return nil, err
	}
	user, err := s.store.GetUserByID(ctx, claims.UserID)
	if errors.Is(err, repository.ErrUserNotFound) {
		return nil, ErrInvalidToken
	}
	if err != nil {
		return nil, err
	}
	if user.Status != "active" {
		return nil, ErrInvalidToken
	}
	return s.buildResponse(user)
}

func (s *Service) ValidateAccessToken(token string) (*Claims, error) {
	return s.parseToken(token, "access")
}

func (s *Service) buildResponse(user *repository.User) (*Response, error) {
	accessToken, err := s.issueToken(user, "access", s.config.AccessTokenExpiry)
	if err != nil {
		return nil, err
	}
	refreshToken, err := s.issueToken(user, "refresh", s.config.RefreshTokenExpiry)
	if err != nil {
		return nil, err
	}
	return &Response{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(s.config.AccessTokenExpiry.Seconds()),
		User: UserInfo{
			ID:          user.ID,
			Email:       user.Email,
			DisplayName: user.DisplayName,
			Role:        user.Role,
			AvatarURL:   user.AvatarURL,
		},
	}, nil
}

func (s *Service) issueToken(user *repository.User, tokenType string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:    user.ID,
		Email:     user.Email,
		Role:      user.Role,
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(s.config.JWTSecret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

func (s *Service) parseToken(rawToken string, expectedType string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return []byte(s.config.JWTSecret), nil
	})
	if err != nil || !token.Valid || claims.TokenType != expectedType {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
