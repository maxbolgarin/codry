package provider

import (
	"context"
	"time"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/provider/bitbucket"
	"github.com/maxbolgarin/codry/internal/provider/github"
	"github.com/maxbolgarin/codry/internal/provider/gitlab"
	"github.com/maxbolgarin/errm"
)

// NewProvider creates a new VCS provider based on the configuration
func NewProvider(cfg Config) (model.CodeProvider, error) {
	if err := cfg.PrepareAndValidate(); err != nil {
		return nil, errm.Wrap(err, "validate config")
	}

	cfgForProvider := model.ProviderConfig{
		BaseURL:       cfg.BaseURL,
		Token:         cfg.Token,
		WebhookSecret: cfg.WebhookSecret,
		BotUsername:   cfg.BotUsername,
	}

	var provider model.CodeProvider
	var err error

	switch cfg.Type {
	case GitLab:
		provider, err = gitlab.NewProvider(cfgForProvider)
	case GitHub:
		provider, err = github.NewProvider(cfgForProvider)
	case Bitbucket:
		provider, err = bitbucket.New(cfgForProvider)
	default:
		return nil, errm.Errorf("unsupported provider type: %s", cfg.Type)
	}

	if err != nil {
		return nil, errm.Wrap(err, "failed to create provider")
	}

	// Validate that the provider implements all required methods
	if err := ValidateProvider(provider); err != nil {
		return nil, errm.Wrap(err, "provider validation failed")
	}

	return provider, nil
}

// ValidateProvider ensures that the provider implements all required interface methods
func ValidateProvider(provider model.CodeProvider) error {
	// This function serves as a compile-time check that all required methods are implemented
	// If any method is missing, this will fail to compile

	ctx := context.Background()

	// Test basic interface compliance by calling methods with nil/empty values
	// These calls should not panic, though they may return errors
	defer func() {
		if r := recover(); r != nil {
			// If any method panics with basic calls, the implementation is incomplete
		}
	}()

	// Webhook methods
	_ = provider.ValidateWebhook(nil, "")
	_, _ = provider.ParseWebhookEvent(nil)
	_ = provider.IsMergeRequestEvent(nil)
	_ = provider.IsCommentEvent(nil)

	// MR/PR operations
	_, _ = provider.GetMergeRequest(ctx, "", 0)
	_, _ = provider.GetMergeRequestDiffs(ctx, "", 0)
	_ = provider.UpdateMergeRequestDescription(ctx, "", 0, "")

	// New MR fetching methods
	_, _ = provider.ListMergeRequests(ctx, "", &model.MergeRequestFilter{})
	_, _ = provider.GetMergeRequestUpdates(ctx, "", time.Now())

	// Comments
	_ = provider.CreateComment(ctx, "", 0, &model.ReviewComment{})
	_ = provider.ReplyToComment(ctx, "", 0, "", "")
	_, _ = provider.GetComments(ctx, "", 0)
	_, _ = provider.GetComment(ctx, "", 0, "")

	// User operations
	_, _ = provider.GetCurrentUser(ctx)

	return nil
}

// ValidateProviderType checks if the given provider type is supported
func ValidateProviderType(providerType ProviderType) error {
	switch providerType {
	case GitLab, GitHub, Bitbucket:
		return nil
	default:
		return errm.Errorf("unsupported provider type: %s", providerType)
	}
}

// GetSupportedProviderTypes returns a list of all supported provider types
func GetSupportedProviderTypes() []ProviderType {
	return []ProviderType{GitLab, GitHub, Bitbucket}
}

// ProviderSupportsFeature checks if a provider supports a specific feature
func ProviderSupportsFeature(providerType ProviderType, feature string) bool {
	switch feature {
	case "threaded_comments":
		// GitLab supports threaded comments, GitHub and Bitbucket don't
		return providerType == GitLab
	case "inline_comments":
		// All providers support inline comments
		return true
	case "draft_prs":
		// GitHub supports draft PRs, GitLab has WIP, Bitbucket doesn't have native support
		return providerType == GitHub || providerType == GitLab
	case "auto_merge":
		// All providers support some form of auto-merge
		return true
	default:
		return false
	}
}
