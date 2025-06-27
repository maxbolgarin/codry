package config

import "errors"

var (
	ErrMissingProviderToken = errors.New("provider token is required")
	ErrMissingProviderType  = errors.New("provider type is required")
	ErrMissingAgentAPIKey   = errors.New("agent API key is required")
	ErrMissingAgentType     = errors.New("agent type is required")
	ErrInvalidProviderType  = errors.New("invalid provider type")
	ErrInvalidAgentType     = errors.New("invalid agent type")
	ErrInvalidServerConfig  = errors.New("invalid server configuration")
)
