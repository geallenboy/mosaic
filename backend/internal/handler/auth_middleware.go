package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/mosaic-app/mosaic/backend/internal/auth"
)

type currentUserKey struct{}

func AuthMiddleware(authSvc interface {
	ValidateAccessToken(token string) (*auth.Claims, error)
}) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r.Header.Get("Authorization"))
			if token == "" {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "请先登录")
				return
			}
			claims, err := authSvc.ValidateAccessToken(token)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "token 无效或已过期")
				return
			}
			ctx := context.WithValue(r.Context(), currentUserKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func CurrentUser(ctx context.Context) (*auth.Claims, bool) {
	claims, ok := ctx.Value(currentUserKey{}).(*auth.Claims)
	return claims, ok
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}
