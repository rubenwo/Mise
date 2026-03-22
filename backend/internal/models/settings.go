package models

import "time"

type OllamaProvider struct {
	ID              int        `json:"id"`
	Name            string     `json:"name"`
	Host            string     `json:"host"`
	Model           string     `json:"model"`
	ProviderType    string     `json:"provider_type"` // "ollama" or "openai_compat"
	Enabled         bool       `json:"enabled"`
	CreatedAt       time.Time  `json:"created_at"`
	HealthStatus    string     `json:"health_status"`
	LastHealthCheck *time.Time `json:"last_health_check"`
	LastError       *string    `json:"last_error"`
	Tags            []string   `json:"tags"`
}

type AppSetting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
