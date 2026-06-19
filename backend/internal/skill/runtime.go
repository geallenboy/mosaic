package skill

import "context"

type Type string

const (
	TypeResearch Type = "research"
	TypeStrategy Type = "strategy"
	TypeCopy     Type = "copy"
	TypeDeck     Type = "deck"
	TypeVisual   Type = "visual"
	TypeWeb      Type = "web"
	TypeVideo    Type = "video"
	TypeOps      Type = "ops"
	TypeDev      Type = "dev"
)

type OutputFormat string

const (
	FormatMarkdown    OutputFormat = "markdown"
	FormatHTML        OutputFormat = "html"
	FormatJSON        OutputFormat = "json"
	FormatImagePrompt OutputFormat = "image_prompt"
	FormatCode        OutputFormat = "code"
)

// ProjectBrief 是所有 Skill 的共享上下文
type ProjectBrief struct {
	ProjectID        string
	Objective        string
	Audience         string
	Positioning      string
	ValueProposition string
	ToneOfVoice      string
	VisualDirection  string
	KeyMessages      []string
	Industry         string
	Market           string
}

type LLMUsage struct {
	Provider     string
	Model        string
	InputTokens  int
	OutputTokens int
}

type Input struct {
	ProjectBrief    *ProjectBrief
	PreviousOutputs map[Type]*Output
	EntryType       string
	UserContext     map[string]any
	RequestID       string
	ProjectID       string
	TaskID          string
}

type Output struct {
	SkillType Type
	EntryType string
	Format    OutputFormat
	Content   string
	Metadata  map[string]any
	LLMUsage  *LLMUsage
}

// Skill 是所有 Skill 的统一接口
type Skill interface {
	Type() Type
	Execute(ctx context.Context, input Input) (*Output, error)
	SupportedEntries() []string
	Dependencies() []Type
}

// Registry 管理所有 Skill 的注册与查找
type Registry struct {
	skills map[Type]Skill
}

func NewRegistry() *Registry {
	return &Registry{skills: make(map[Type]Skill)}
}

func (r *Registry) Register(s Skill) {
	r.skills[s.Type()] = s
}

func (r *Registry) Get(t Type) (Skill, bool) {
	s, ok := r.skills[t]
	return s, ok
}

func (r *Registry) All() []Skill {
	result := make([]Skill, 0, len(r.skills))
	for _, s := range r.skills {
		result = append(result, s)
	}
	return result
}
