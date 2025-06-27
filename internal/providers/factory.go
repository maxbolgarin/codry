package providers

import (
	"fmt"

	"github.com/maxbolgarin/codry/internal/models"
	"github.com/maxbolgarin/codry/internal/providers/bitbucket"
	"github.com/maxbolgarin/codry/internal/providers/github"
	"github.com/maxbolgarin/codry/internal/providers/gitlab"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

// SupportedProviderTypes defines the supported VCS provider types
const (
	ProviderTypeGitLab    = "gitlab"
	ProviderTypeGitHub    = "github"
	ProviderTypeBitbucket = "bitbucket"
	// Add more provider types here in the future
	// ProviderTypeCodecommit = "codecommit"
	// ProviderTypeAzureDevOps = "azuredevops"
)

// NewProvider creates a new VCS provider based on the configuration
func NewProvider(cfg models.ProviderConfig, logger logze.Logger) (models.CodeProvider, error) {
	switch cfg.Type {
	case ProviderTypeGitLab:
		return gitlab.NewProvider(cfg, logger)
	case ProviderTypeGitHub:
		return github.NewProvider(cfg, logger)
	case ProviderTypeBitbucket:
		return bitbucket.NewProvider(cfg, logger)
	default:
		return nil, errm.New(fmt.Sprintf("unsupported provider type: %s", cfg.Type))
	}
}

// GetSupportedProviderTypes returns a list of supported provider types
func GetSupportedProviderTypes() []string {
	return []string{
		ProviderTypeGitLab,
		ProviderTypeGitHub,
		ProviderTypeBitbucket,
		// Add more as they are implemented
	}
}

// ValidateProviderType checks if the given provider type is supported
func ValidateProviderType(providerType string) error {
	supportedTypes := GetSupportedProviderTypes()
	for _, supportedType := range supportedTypes {
		if providerType == supportedType {
			return nil
		}
	}
	return errm.New(fmt.Sprintf("unsupported provider type: %s, supported types: %v", providerType, supportedTypes))
}
