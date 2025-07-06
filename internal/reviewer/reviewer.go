package reviewer

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

	"github.com/maxbolgarin/abstract"
	"github.com/maxbolgarin/codry/internal/agent"
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
	"github.com/panjf2000/ants/v2"
)

// Reviewer implements the ReviewService interface
type Reviewer struct {
	provider interfaces.CodeProvider
	agent    *agent.Agent
	pool     *ants.Pool
	parser   *diffParser

	cfg Config
	log logze.Logger

	// Enhanced context gathering
	contextGatherer *ContextGatherer

	// Quality scoring for prioritizing issues
	qualityScorer *QualityScorer

	// Track processed MRs and reviewed files
	processedMRs *abstract.SafeMapOfMaps[string, string, string]
}

// New creates a new reviewer
func New(cfg Config, provider interfaces.CodeProvider, agent *agent.Agent) (*Reviewer, error) {
	pool, err := ants.NewPool(100)
	if err != nil {
		return nil, errm.Wrap(err, "failed to create ants pool")
	}

	if cfg.Language == "" {
		cfg.Language = model.LanguageEnglish
	}

	s := &Reviewer{
		provider:        provider,
		agent:           agent,
		cfg:             cfg,
		log:             logze.With("component", "reviewer"),
		pool:            pool,
		parser:          newDiffParser(),
		contextGatherer: NewContextGatherer(provider),
		qualityScorer:   NewQualityScorer(DefaultQualityScoringConfig()),
		processedMRs:    abstract.NewSafeMapOfMaps[string, string, string](),
	}

	return s, nil
}

// HandleWebhook processes incoming webhook events and routes them appropriately
func (s *Reviewer) HandleEvent(ctx context.Context, event *model.CodeEvent) error {
	log := s.log.WithFields(
		"event_type", event.Type,
		"action", event.Action,
		"project_id", event.ProjectID,
		"user", event.User.Username,
	)

	log.Info("processing event")

	switch {
	case s.provider.IsMergeRequestEvent(event):
		return s.pool.Submit(func() {
			// TODO: add error handling
			err := s.ReviewMergeRequest(ctx, event.ProjectID, event.MergeRequest)
			if err != nil {
				log.Error("error processing merge request event", "error", err)
			}
		})

	// case s.provider.IsCommentEvent(event):
	// 	return s.pool.Submit(func() {
	// 		// TODO: add error handling
	// 		s.processCommentEvent(ctx, event, log)
	// 	})

	default:
		log.Debug("unhandled webhook event type")
		return nil
	}
}

func (s *Reviewer) isCodeFile(filePath string) bool {
	if s.cfg.FileFilter.IncludeOnlyCode {
		ext := strings.ToLower(filepath.Ext(filePath))
		return slices.Contains(s.cfg.FileFilter.AllowedExtensions, ext)
	}
	return true
}

func (s *Reviewer) isExcludedPath(filePath string) bool {
	for _, pattern := range s.cfg.FileFilter.ExcludedPaths {
		if matched, _ := filepath.Match(pattern, filePath); matched {
			return true
		}
		if strings.Contains(filePath, pattern) {
			return true
		}
	}
	return false
}
