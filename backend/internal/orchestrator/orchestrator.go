package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/mosaic-app/mosaic/backend/internal/skill"
	"golang.org/x/sync/errgroup"
)

// Phase 表示项目执行阶段
type Phase string

const (
	PhaseBriefGeneration   Phase = "brief_generation"
	PhaseSkillExecution    Phase = "skill_execution"
	PhaseConsistencyCheck  Phase = "consistency_check"
	PhaseDone              Phase = "done"
	PhaseFailed            Phase = "failed"
)

// TaskStatus 表示单个任务状态
type TaskStatus struct {
	SkillType   skill.Type
	Status      string // pending | running | done | failed
	ErrorMsg    string
	StartedAt   *time.Time
	CompletedAt *time.Time
}

// ProjectStatus 是项目执行状态（SSE 推送的内容）
type ProjectStatus struct {
	ProjectID string
	Phase     Phase
	Progress  int
	Tasks     []TaskStatus
}

// Orchestrator 负责项目工作流调度
type Orchestrator struct {
	registry *skill.Registry

	mu       sync.RWMutex
	statuses map[string]*projectState // projectID -> state
}

type projectState struct {
	status  ProjectStatus
	outputs map[skill.Type]*skill.Output
}

func New(registry *skill.Registry) *Orchestrator {
	return &Orchestrator{
		registry: registry,
		statuses: make(map[string]*projectState),
	}
}

// StartAsync 异步启动项目执行，立即返回
func (o *Orchestrator) StartAsync(ctx context.Context, projectID string, brief *skill.ProjectBrief, selectedEntries []string) {
	state := &projectState{
		status: ProjectStatus{
			ProjectID: projectID,
			Phase:     PhaseBriefGeneration,
			Progress:  0,
		},
		outputs: make(map[skill.Type]*skill.Output),
	}
	o.mu.Lock()
	o.statuses[projectID] = state
	o.mu.Unlock()

	go func() {
		execCtx := context.Background() // 独立 context，不受 HTTP 请求 cancel 影响
		if err := o.execute(execCtx, projectID, brief, selectedEntries); err != nil {
			slog.Error("orchestrator execute failed", "project_id", projectID, "err", err)
			o.setPhase(projectID, PhaseFailed)
		}
	}()
}

// GetStatus 查询当前执行状态
func (o *Orchestrator) GetStatus(projectID string) (*ProjectStatus, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	state, ok := o.statuses[projectID]
	if !ok {
		return nil, false
	}
	statusCopy := state.status
	return &statusCopy, true
}

func (o *Orchestrator) execute(ctx context.Context, projectID string, brief *skill.ProjectBrief, selectedEntries []string) error {
	// 确定需要执行的 Skill 列表（根据 selectedEntries）
	skillsToRun := o.resolveSkills(selectedEntries)
	if len(skillsToRun) == 0 {
		return fmt.Errorf("no skills resolved for entries: %v", selectedEntries)
	}

	// 初始化任务状态
	taskStatuses := make([]TaskStatus, 0, len(skillsToRun))
	for _, s := range skillsToRun {
		taskStatuses = append(taskStatuses, TaskStatus{
			SkillType: s.Type(),
			Status:    "pending",
		})
	}
	o.updateTasks(projectID, taskStatuses)
	o.setPhase(projectID, PhaseSkillExecution)

	// 按依赖关系分层执行
	layers := topologicalSort(skillsToRun)
	totalLayers := len(layers)
	completedLayers := 0

	state := o.getState(projectID)

	for _, layer := range layers {
		g, gCtx := errgroup.WithContext(ctx)

		for _, s := range layer {
			s := s
			g.Go(func() error {
				o.updateTaskStatus(projectID, s.Type(), "running", "")

				input := skill.Input{
					ProjectBrief:    brief,
					PreviousOutputs: state.outputs,
					ProjectID:       projectID,
				}

				output, err := s.Execute(gCtx, input)
				if err != nil {
					slog.ErrorContext(gCtx, "skill_failed",
						"skill_type", s.Type(),
						"project_id", projectID,
						"err", err,
					)
					o.updateTaskStatus(projectID, s.Type(), "failed", err.Error())
					return nil // 单个 Skill 失败不中断其他 Skill
				}

				o.mu.Lock()
				if st := o.statuses[projectID]; st != nil {
					st.outputs[s.Type()] = output
				}
				o.mu.Unlock()

				o.updateTaskStatus(projectID, s.Type(), "done", "")
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return err
		}

		completedLayers++
		progress := (completedLayers * 90) / totalLayers
		o.setProgress(projectID, progress)
	}

	o.setProgress(projectID, 100)
	o.setPhase(projectID, PhaseDone)
	slog.Info("project_completed", "project_id", projectID)
	return nil
}

// resolveSkills 根据所选入口确定需要运行的 Skill
func (o *Orchestrator) resolveSkills(selectedEntries []string) []skill.Skill {
	entrySet := make(map[string]bool, len(selectedEntries))
	for _, e := range selectedEntries {
		entrySet[e] = true
	}

	seen := make(map[skill.Type]bool)
	var result []skill.Skill

	for _, s := range o.registry.All() {
		for _, entry := range s.SupportedEntries() {
			if entrySet[entry] && !seen[s.Type()] {
				seen[s.Type()] = true
				result = append(result, s)
				break
			}
		}
	}

	// 添加所有必要的依赖 Skill（即使它们的入口没被选中）
	changed := true
	for changed {
		changed = false
		for _, s := range result {
			for _, dep := range s.Dependencies() {
				if !seen[dep] {
					if depSkill, ok := o.registry.Get(dep); ok {
						seen[dep] = true
						result = append(result, depSkill)
						changed = true
					}
				}
			}
		}
	}
	return result
}

// topologicalSort 将 Skill 列表按依赖关系分层
func topologicalSort(skills []skill.Skill) [][]skill.Skill {
	skillMap := make(map[skill.Type]skill.Skill)
	for _, s := range skills {
		skillMap[s.Type()] = s
	}

	inDegree := make(map[skill.Type]int)
	for _, s := range skills {
		if _, ok := inDegree[s.Type()]; !ok {
			inDegree[s.Type()] = 0
		}
		for _, dep := range s.Dependencies() {
			if _, ok := skillMap[dep]; ok {
				inDegree[s.Type()]++
			}
		}
	}

	var layers [][]skill.Skill
	remaining := make(map[skill.Type]skill.Skill)
	for _, s := range skills {
		remaining[s.Type()] = s
	}

	for len(remaining) > 0 {
		var layer []skill.Skill
		for t, s := range remaining {
			if inDegree[t] == 0 {
				layer = append(layer, s)
			}
		}
		if len(layer) == 0 {
			// 存在循环依赖，将剩余 Skill 全部放入最后一层
			for _, s := range remaining {
				layer = append(layer, s)
			}
			layers = append(layers, layer)
			break
		}
		layers = append(layers, layer)
		for _, s := range layer {
			delete(remaining, s.Type())
			// 更新 in-degree
			for t2, s2 := range remaining {
				for _, dep := range s2.Dependencies() {
					if dep == s.Type() {
						inDegree[t2]--
					}
				}
			}
		}
	}
	return layers
}

func (o *Orchestrator) getState(projectID string) *projectState {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.statuses[projectID]
}

func (o *Orchestrator) setPhase(projectID string, phase Phase) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if s := o.statuses[projectID]; s != nil {
		s.status.Phase = phase
	}
}

func (o *Orchestrator) setProgress(projectID string, progress int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if s := o.statuses[projectID]; s != nil {
		s.status.Progress = progress
	}
}

func (o *Orchestrator) updateTasks(projectID string, tasks []TaskStatus) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if s := o.statuses[projectID]; s != nil {
		s.status.Tasks = tasks
	}
}

func (o *Orchestrator) updateTaskStatus(projectID string, skillType skill.Type, status, errMsg string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	state := o.statuses[projectID]
	if state == nil {
		return
	}
	now := time.Now()
	for i := range state.status.Tasks {
		if state.status.Tasks[i].SkillType == skillType {
			state.status.Tasks[i].Status = status
			state.status.Tasks[i].ErrorMsg = errMsg
			if status == "running" {
				state.status.Tasks[i].StartedAt = &now
			} else if status == "done" || status == "failed" {
				state.status.Tasks[i].CompletedAt = &now
			}
			break
		}
	}
}
