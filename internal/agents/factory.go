package agents

import (
	"context"
	"fmt"

	"github.com/maxbolgarin/codry/internal/agents/claude"
	"github.com/maxbolgarin/codry/internal/agents/gemini"
	"github.com/maxbolgarin/codry/internal/agents/openai"
	"github.com/maxbolgarin/codry/internal/config"
	"github.com/maxbolgarin/codry/internal/models"
	"github.com/maxbolgarin/errm"
)

// SupportedAgentTypes defines the supported AI agent types
const (
	AgentTypeGemini = "gemini"
	AgentTypeOpenAI = "openai"
	AgentTypeClaude = "claude"
	// Add more agent types here in the future
	// AgentTypeOllama = "ollama"
	// AgentTypeCohere = "cohere"
)

// NewAgent creates a new AI agent based on the configuration
func NewAgent(ctx context.Context, cfg config.AgentConfig) (models.AIAgent, error) {
	switch cfg.Type {
	case AgentTypeGemini:
		return gemini.NewAgent(ctx, cfg)
	case AgentTypeOpenAI:
		return openai.NewAgent(ctx, cfg)
	case AgentTypeClaude:
		return claude.NewAgent(ctx, cfg)
	default:
		return nil, errm.New(fmt.Sprintf("unsupported agent type: %s", cfg.Type))
	}
}

// GetSupportedAgentTypes returns a list of supported agent types
func GetSupportedAgentTypes() []string {
	return []string{
		AgentTypeGemini,
		AgentTypeOpenAI,
		AgentTypeClaude,
		// Add more as they are implemented
	}
}

// ValidateAgentType checks if the given agent type is supported
func ValidateAgentType(agentType string) error {
	supportedTypes := GetSupportedAgentTypes()
	for _, supportedType := range supportedTypes {
		if agentType == supportedType {
			return nil
		}
	}
	return errm.New(fmt.Sprintf("unsupported agent type: %s, supported types: %v", agentType, supportedTypes))
}
