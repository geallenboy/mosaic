package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/mosaic-app/mosaic/backend/internal/auth"
	appconfig "github.com/mosaic-app/mosaic/backend/internal/config"
	"github.com/mosaic-app/mosaic/backend/internal/handler"
	"github.com/mosaic-app/mosaic/backend/internal/llm"
	"github.com/mosaic-app/mosaic/backend/internal/orchestrator"
	"github.com/mosaic-app/mosaic/backend/internal/repository"
	"github.com/mosaic-app/mosaic/backend/internal/skill"
)

func main() {
	cfg := appconfig.Load()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	dbCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := repository.OpenPostgres(dbCtx, cfg.Database.URL)
	if err != nil {
		slog.Error("database connection failed", "err", err)
		os.Exit(1)
	}
	defer db.Close()
	store := repository.NewPostgresStore(db)

	// LLM Provider
	llmProvider := llm.NewOpenAI(cfg.LLM.APIKey, cfg.LLM.Model)

	// Skill Registry（V0.1：注册 4 个核心 Skill）
	registry := skill.NewRegistry()
	registry.Register(skill.NewResearchSkill(llmProvider))
	registry.Register(skill.NewStrategySkill(llmProvider))
	registry.Register(skill.NewCopySkill(llmProvider))
	registry.Register(skill.NewDeckSkill(llmProvider))

	// 编排器
	orch := orchestrator.NewWithStore(registry, store)
	authService := auth.NewService(store, auth.Config{
		JWTSecret:          cfg.Auth.JWTSecret,
		AccessTokenExpiry:  cfg.Auth.AccessTokenExpiry,
		RefreshTokenExpiry: cfg.Auth.RefreshTokenExpiry,
	})

	// Handlers
	authHandler := handler.NewAuthHandler(authService)
	projectHandler := handler.NewProjectHandler(orch, store)

	// 路由
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)

	// 健康检查
	r.Get("/health", handler.HealthCheck)

	// 用户侧 API
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/register", authHandler.Register)
		r.Post("/auth/login", authHandler.Login)
		r.Post("/auth/refresh", authHandler.Refresh)

		r.Group(func(r chi.Router) {
			r.Use(handler.AuthMiddleware(authService))
			// V0.1 项目接口（简化版，完整项目 API 后续添加）
			r.Get("/projects", projectHandler.ListProjects)
			r.Post("/projects/start", projectHandler.CreateAndStart)
			r.Get("/projects/{id}", projectHandler.GetProject)
			r.Get("/projects/{id}/progress", projectHandler.GetProgress)
			r.Get("/projects/{id}/deliverables", projectHandler.ListDeliverables)
			r.Get("/deliverables/{id}", projectHandler.GetDeliverable)
		})
	})

	// Admin API（V0.1 占位）
	r.Route("/admin/api/v1", func(r chi.Router) {
		r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"message":"admin pong"}`))
		})
	})

	slog.Info("server starting", "addr", cfg.Server.Addr)
	if err := http.ListenAndServe(cfg.Server.Addr, r); err != nil {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}
