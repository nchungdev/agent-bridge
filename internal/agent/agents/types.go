package agents

// ModelInfo describes a model supported by an agent.
type ModelInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Tier     string `json:"tier"`     // "fast", "medium", "smart"
	Provider string `json:"provider"` // "google", "anthropic", "openai"
}

// AgentInfo describes an available agent for the frontend.
type AgentInfo struct {
	ID          string      `json:"id"`
	DisplayName string      `json:"display_name"`
	Available   bool        `json:"available"`
	Models      []ModelInfo `json:"models"`
	Default     string      `json:"default_model"`
}
