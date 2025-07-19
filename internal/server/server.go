package server

import (
	"context"
	"net/http"

	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/codry/internal/reviewer"
	"github.com/maxbolgarin/erro"
	"github.com/maxbolgarin/logze/v2"
	"github.com/maxbolgarin/servex/v2"
)

// Server handles webhook requests from VCS providers
type Server struct {
	provider interfaces.CodeProvider
	reviewer *reviewer.Reviewer
	config   Config
	log      logze.Logger
	server   *servex.Server
}

// New creates a new webhook handler
func New(cfg Config, provider interfaces.CodeProvider, reviewer *reviewer.Reviewer) (*Server, error) {
	if err := cfg.PrepareAndValidate(); err != nil {
		return nil, erro.Wrap(err, "validate config")
	}

	log := logze.With("module", "server")

	server, err := servex.NewServer(
		servex.WithReadTimeout(cfg.Timeout),
		servex.WithIdleTimeout(cfg.Timeout*2),
		servex.WithLogger(log),
		servex.WithHealthEndpoint(),
		servex.WithDefaultMetrics(),
		servex.WithCertificate(cfg.Certificate),
	)
	if err != nil {
		return nil, erro.Wrap(err, "failed to create server")
	}

	h := &Server{
		provider: provider,
		reviewer: reviewer,
		config:   cfg,
		log:      log,
		server:   server,
	}

	server.HandleFunc(cfg.Endpoint, h.handleWebhook)

	return h, nil
}

// Start starts the webhook server
func (h *Server) Start(ctx context.Context) error {
	if h.config.EnableHTTPS {
		return h.server.StartHTTPS(h.config.Address)
	}
	return h.server.StartHTTP(h.config.Address)
}

// Stop stops the webhook server
func (h *Server) Stop(ctx context.Context) error {
	return h.server.Shutdown(ctx)
}

// handleWebhook handles incoming webhook requests
func (h *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := servex.NewContext(w, r)

	body, err := ctx.Read()
	if err != nil {
		ctx.BadRequest(err, "failed to read webhook body")
		return
	}

	// Get token from headers (provider-specific)
	token := h.getAuthFromHeaders(r)

	// Validate webhook signature
	if err := h.provider.ValidateWebhook(body, token); err != nil {
		ctx.Unauthorized(err, "webhook validation failed")
		return
	}

	// Parse webhook event
	event, err := h.provider.ParseWebhookEvent(body)
	if err != nil {
		ctx.BadRequest(err, "failed to parse webhook event")
		return
	}

	// Check if this is a merge request event that should be processed
	if !h.provider.IsMergeRequestEvent(event) {
		h.log.Debug("ignoring non-merge request event")
		ctx.Response(http.StatusOK)
		return
	}

	h.log.Info("received merge request event", "mr_title", event.MergeRequest.Title, "action", event.Action)

	// Pass event to review service - it will handle all the processing logic
	if err := h.reviewer.HandleEvent(ctx, event); err != nil {
		ctx.InternalServerError(err, "failed to handle event")
		return
	}
}

// getAuthFromHeaders extracts auth token from request headers
func (h *Server) getAuthFromHeaders(r *http.Request) string {
	// Try different header names used by different providers
	for _, header := range authHeaders {
		if value := r.Header.Get(header); value != "" {
			return value
		}
	}
	return ""
}
