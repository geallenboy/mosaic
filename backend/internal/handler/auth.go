package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/mosaic-app/mosaic/backend/internal/auth"
)

type authService interface {
	Register(ctx context.Context, req auth.RegisterRequest) (*auth.Response, error)
	Login(ctx context.Context, req auth.LoginRequest) (*auth.Response, error)
	Refresh(ctx context.Context, refreshToken string) (*auth.Response, error)
	ValidateAccessToken(token string) (*auth.Claims, error)
}

type AuthHandler struct {
	auth authService
}

func NewAuthHandler(authSvc authService) *AuthHandler {
	return &AuthHandler{auth: authSvc}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "请求格式错误")
		return
	}
	if req.Email == "" || len(req.Password) < 8 || strings.TrimSpace(req.DisplayName) == "" {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "email, password, display_name 为必填项")
		return
	}
	resp, err := h.auth.Register(r.Context(), auth.RegisterRequest{
		Email:       req.Email,
		Password:    req.Password,
		DisplayName: req.DisplayName,
	})
	if errors.Is(err, auth.ErrEmailExists) {
		writeError(w, http.StatusConflict, "EMAIL_EXISTS", "邮箱已注册")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "REGISTER_FAILED", "注册失败")
		return
	}
	writeJSON(w, http.StatusCreated, authPayload(resp))
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "请求格式错误")
		return
	}
	resp, err := h.auth.Login(r.Context(), auth.LoginRequest{
		Email:    req.Email,
		Password: req.Password,
	})
	if errors.Is(err, auth.ErrInvalidCredentials) {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "邮箱或密码错误")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "LOGIN_FAILED", "登录失败")
		return
	}
	writeJSON(w, http.StatusOK, authPayload(resp))
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "请求格式错误")
		return
	}
	resp, err := h.auth.Refresh(r.Context(), req.RefreshToken)
	if errors.Is(err, auth.ErrInvalidToken) {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "refresh token 无效或已过期")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "REFRESH_FAILED", "刷新 token 失败")
		return
	}
	writeJSON(w, http.StatusOK, authPayload(resp))
}

func authPayload(resp *auth.Response) map[string]any {
	return map[string]any{
		"data": map[string]any{
			"access_token":  resp.AccessToken,
			"refresh_token": resp.RefreshToken,
			"expires_in":    resp.ExpiresIn,
			"user":          resp.User,
		},
	}
}
