package webhook

import (
	"context"
	"io"
	"net/http"

	"github.com/maxbolgarin/codry/internal/config"
	"github.com/maxbolgarin/codry/internal/models"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

// Handler handles webhook requests from VCS providers
type Handler struct {
	provider      models.CodeProvider
	reviewService models.ReviewService
	config        *config.Config
	logger        logze.Logger
	server        *http.Server
}

// NewHandler creates a new webhook handler
func NewHandler(
	provider models.CodeProvider,
	reviewService models.ReviewService,
	cfg *config.Config,
	logger logze.Logger,
) *Handler {
	return &Handler{
		provider:      provider,
		reviewService: reviewService,
		config:        cfg,
		logger:        logger,
		server: &http.Server{
			Addr:         cfg.Server.Address,
			ReadTimeout:  cfg.Server.Timeout,
			WriteTimeout: cfg.Server.Timeout,
			IdleTimeout:  cfg.Server.Timeout * 2,
		},
	}
}

// Start starts the webhook server
func (h *Handler) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc(h.config.Server.Endpoint, h.handleWebhook)
	mux.HandleFunc("/health", h.handleHealth)
	h.server.Handler = mux

	h.logger.Info("webhook server starting",
		"address", h.config.Server.Address,
		"endpoint", h.config.Server.Endpoint,
	)

	// Start server in goroutine
	go func() {
		if err := h.server.ListenAndServe(); err != nil && !errm.Is(err, http.ErrServerClosed) {
			h.logger.Err(err, "webhook server failed to start")
		}
	}()

	// Handle graceful shutdown
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), h.config.Server.Timeout)
		defer cancel()

		if err := h.server.Shutdown(shutdownCtx); err != nil {
			h.logger.Err(err, "webhook server failed to shutdown gracefully")
		}
	}()

	return nil
}

// handleWebhook handles incoming webhook requests
func (h *Handler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Err(err, "failed to read webhook body")
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Get signature from headers (provider-specific)
	signature := h.getSignatureFromHeaders(r)

	// Validate webhook signature
	if err := h.provider.ValidateWebhook(body, signature); err != nil {
		h.logger.Warn("webhook validation failed", "error", err.Error())
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse webhook event
	event, err := h.provider.ParseWebhookEvent(body)
	if err != nil {
		h.logger.Err(err, "failed to parse webhook event")
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Log the event
	log := h.logger.WithFields(
		"event_type", event.Type,
		"action", event.Action,
		"project_id", event.ProjectID,
		"user", event.User.Username,
	)

	// Check if we should process this event
	if !h.shouldProcessEvent(event, log) {
		log.Debug("event ignored")
		w.WriteHeader(http.StatusOK)
		return
	}

	log.Info("processing webhook event", "mr_title", event.MergeRequest.Title)

	// Process asynchronously to avoid webhook timeouts
	go h.processWebhookEvent(context.Background(), event, log)

	w.WriteHeader(http.StatusOK)
}

// handleHealth handles health check requests
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// getSignatureFromHeaders extracts signature from request headers
func (h *Handler) getSignatureFromHeaders(r *http.Request) string {
	// Try different header names used by different providers
	headers := []string{
		"X-Gitlab-Token",      // GitLab
		"X-Hub-Signature-256", // GitHub
		"X-Hub-Signature",     // GitHub (legacy)
		"X-Hook-UUID",         // Bitbucket webhook signature
		"X-Request-UUID",      // Bitbucket alternative
		"Authorization",       // Generic
	}

	for _, header := range headers {
		if value := r.Header.Get(header); value != "" {
			return value
		}
	}

	return ""
}

// shouldProcessEvent determines if an event should be processed
func (h *Handler) shouldProcessEvent(event *models.WebhookEvent, log logze.Logger) bool {
	// Only process merge request/pull request events
	if event.Type != "merge_request" && event.Type != "pull_request" && event.Type != "pullrequest" {
		return false
	}

	// Check for relevant actions
	relevantActions := []string{
		"open", "opened", "reopen", "reopened", "update", "updated", "synchronize",
		"review_requested", // GitHub: when reviewer is added
		"ready_for_review", // GitHub: when PR is marked ready
		"created",          // Bitbucket: when PR is created
		"updated",          // Bitbucket: when PR is updated
		"reviewer_added",   // Bitbucket: when reviewer is added
	}
	isRelevantAction := false
	for _, action := range relevantActions {
		if event.Action == action {
			isRelevantAction = true
			break
		}
	}

	if !isRelevantAction {
		log.Debug("ignoring irrelevant action", "action", event.Action)
		return false
	}

	// Don't process events from the bot itself to avoid loops
	if event.User.Username == h.config.Provider.BotUsername {
		log.Debug("ignoring event from bot user")
		return false
	}

	// Special handling for reviewer-based triggers
	if event.Action == "review_requested" || event.Action == "reviewer_added" {
		// Check if the bot was added as a reviewer
		botIsReviewer := false
		for _, reviewer := range event.MergeRequest.Reviewers {
			if reviewer.Username == h.config.Provider.BotUsername {
				botIsReviewer = true
				break
			}
		}

		if !botIsReviewer {
			log.Debug("bot not in reviewers list for review_requested action")
			return false
		}

		log.Info("bot was added as reviewer, triggering review")
		return true
	}

	// Check if MR/PR should be processed
	if !h.reviewService.ShouldProcessMergeRequest(context.Background(), event.MergeRequest) {
		log.Debug("merge request should not be processed")
		return false
	}

	return true
}

// processWebhookEvent processes a webhook event asynchronously
func (h *Handler) processWebhookEvent(ctx context.Context, event *models.WebhookEvent, log logze.Logger) {
	// Get detailed MR information and diffs
	mr, err := h.provider.GetMergeRequest(ctx, event.ProjectID, event.MergeRequest.IID)
	if err != nil {
		log.Err(err, "failed to get merge request details")
		return
	}

	// Get file diffs
	diffs, err := h.provider.GetMergeRequestDiffs(ctx, event.ProjectID, event.MergeRequest.IID)
	if err != nil {
		log.Err(err, "failed to get merge request diffs")
		return
	}

	// Create review request
	reviewRequest := &models.ReviewRequest{
		ProjectID:    event.ProjectID,
		MergeRequest: mr,
		Changes:      diffs,
		CommitSHA:    mr.SHA,
	}

	// Process the review
	result, err := h.reviewService.ProcessMergeRequest(ctx, reviewRequest)
	if err != nil {
		log.Err(err, "failed to process merge request")
		return
	}

	// Log results
	if result.Success {
		log.Info("successfully processed merge request",
			"processed_files", result.ProcessedFiles,
			"comments_created", result.CommentsCreated,
			"description_updated", result.DescriptionUpdated,
		)
	} else {
		log.Error("merge request processing completed with errors",
			"error_count", len(result.Errors),
			"processed_files", result.ProcessedFiles,
		)
		for _, err := range result.Errors {
			log.Error("processing error", "error", err.Error())
		}
	}
}
