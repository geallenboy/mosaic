package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/mosaic-app/mosaic/backend/internal/orchestrator"
	"github.com/mosaic-app/mosaic/backend/internal/repository"
	"github.com/mosaic-app/mosaic/backend/internal/skill"
)

type projectStarter interface {
	StartAsync(ctx context.Context, projectID string, brief *skill.ProjectBrief, selectedEntries []string)
	GetStatus(projectID string) (*orchestrator.ProjectStatus, bool)
}

type projectCreator interface {
	CreateProject(ctx context.Context, params repository.CreateProjectParams) (string, error)
}

type ProjectHandler struct {
	orch     projectStarter
	projects projectCreator
}

func NewProjectHandler(orch projectStarter, projects projectCreator) *ProjectHandler {
	return &ProjectHandler{orch: orch, projects: projects}
}

type createProjectReq struct {
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	Industry        string   `json:"industry"`
	Market          string   `json:"market"`
	TargetAudience  string   `json:"target_audience"`
	Goal            string   `json:"goal"`
	BudgetRange     string   `json:"budget_range"`
	StyleKeywords   []string `json:"style_keywords"`
	SelectedEntries []string `json:"selected_entries"`
}

// CreateAndStart 创建项目并立即启动（V0.1 简化版：无数据库，直接执行）
func (h *ProjectHandler) CreateAndStart(w http.ResponseWriter, r *http.Request) {
	var req createProjectReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "请求格式错误")
		return
	}

	if req.Name == "" || req.Goal == "" || len(req.SelectedEntries) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "name, goal, selected_entries 为必填项")
		return
	}

	projectID, err := h.projects.CreateProject(r.Context(), repository.CreateProjectParams{
		Name:            req.Name,
		Type:            req.Type,
		Industry:        req.Industry,
		Market:          req.Market,
		TargetAudience:  req.TargetAudience,
		Goal:            req.Goal,
		BudgetRange:     req.BudgetRange,
		StyleKeywords:   req.StyleKeywords,
		SelectedEntries: req.SelectedEntries,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "CREATE_PROJECT_FAILED", "项目创建失败")
		return
	}

	// V0.1 简化：直接从请求构建 Brief，后续接入 LLM Brief 生成
	brief := &skill.ProjectBrief{
		ProjectID:        projectID,
		Objective:        req.Goal,
		Audience:         req.TargetAudience,
		Positioning:      req.Name,
		ValueProposition: req.Goal,
		ToneOfVoice:      "真诚、专业、有温度",
		VisualDirection:  "简洁现代",
		Industry:         req.Industry,
		Market:           req.Market,
	}

	// 异步启动执行
	h.orch.StartAsync(r.Context(), projectID, brief, req.SelectedEntries)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"data": map[string]any{
			"project_id": projectID,
			"status":     "running",
			"message":    "项目已开始执行，通过 SSE 接口获取实时进度",
		},
	})
}

// GetProgress SSE 实时进度推送
func (h *ProjectHandler) GetProgress(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			status, found := h.orch.GetStatus(projectID)
			if !found {
				fmt.Fprintf(w, "event: error\ndata: {\"message\":\"project not found\"}\n\n")
				flusher.Flush()
				return
			}

			data, _ := json.Marshal(status)
			fmt.Fprintf(w, "event: progress\ndata: %s\n\n", data)
			flusher.Flush()

			if status.Phase == orchestrator.PhaseDone || status.Phase == orchestrator.PhaseFailed {
				fmt.Fprintf(w, "event: close\ndata: {}\n\n")
				flusher.Flush()
				return
			}
		}
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, errCode, msg string) {
	writeJSON(w, code, map[string]any{
		"error": map[string]any{
			"code":    errCode,
			"message": msg,
		},
	})
}
