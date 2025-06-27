package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/maxbolgarin/codry/internal/agent"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

const (
	gitlabBaseURL = "https://gitlab.158-160-60-159.sslip.io/"
)

type Config struct {
	Token         string `yaml:"token" env:"GITLAB_TOKEN"`
	GitlabBaseURL string `yaml:"gitlab_base_url" env:"GITLAB_BASE_URL"`
	BotUsername   string `yaml:"bot_username" env:"GITLAB_BOT_USERNAME"`
	WebhookSecret string `yaml:"webhook_secret" env:"GITLAB_WEBHOOK_SECRET"`

	WebhookAddr     string `yaml:"webhook_addr" env:"GITLAB_WEBHOOK_ADDR"`
	WebhookEndpoint string `yaml:"webhook_endpoint" env:"GITLAB_WEBHOOK_ENDPOINT"`

	IntervalToWaitLimits time.Duration `yaml:"interval_to_wait_limits" env:"GITLAB_INTERVAL_TO_WAIT_LIMITS"`
}

// Структура для парсинга полезной нагрузки веб-хука от GitLab
type WebhookPayload struct {
	ObjectKind string `json:"object_kind"`
	EventType  string `json:"event_type"`
	User       struct {
		Username string `json:"username"`
	} `json:"user"`
	Project struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"project"`
	ObjectAttributes struct {
		IID          int    `json:"iid"`
		Action       string `json:"action"`
		State        string `json:"state"`
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
		URL          string `json:"url"`
		Title        string `json:"title"`
		AuthorID     int    `json:"author_id"`
		ReviewerIDs  []int  `json:"reviewer_ids"`
	} `json:"object_attributes"`
}

type Agent interface {
	GenerateDescription(ctx context.Context, fullDiff string) (string, error)
	ReviewCode(ctx context.Context, filePath, diff string) (string, error)
}

type Client struct {
	cli   *gitlab.Client
	srv   *http.Server
	agent Agent

	log logze.Logger
	cfg Config

	// Отслеживание обработанных MR
	processedMRs   map[string]bool
	processedMRsMu sync.RWMutex

	// Отслеживание проанализированных файлов (MRKey -> filePath -> fileHash)
	reviewedFiles   map[string]map[string]string
	reviewedFilesMu sync.RWMutex
}

func New(cfg Config, llmAgent Agent) (*Client, error) {
	if cfg.Token == "" {
		return nil, errm.New("token is required")
	}
	if cfg.GitlabBaseURL == "" {
		cfg.GitlabBaseURL = gitlabBaseURL
	}
	if cfg.IntervalToWaitLimits == 0 {
		cfg.IntervalToWaitLimits = 1 * time.Minute
	}
	cli, err := gitlab.NewClient(cfg.Token, gitlab.WithBaseURL(cfg.GitlabBaseURL))
	if err != nil {
		return nil, errm.Wrap(err, "failed to create gitlab client")
	}

	return &Client{
		cli:   cli,
		log:   logze.Default(),
		cfg:   cfg,
		agent: llmAgent,
		srv: &http.Server{
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
		processedMRs:  make(map[string]bool),
		reviewedFiles: make(map[string]map[string]string),
	}, nil
}

func (c *Client) StartWebhookServer(ctx context.Context) error {
	if c.cfg.WebhookEndpoint == "" {
		c.cfg.WebhookEndpoint = "/webhook"
	}
	if c.cfg.WebhookAddr == "" {
		c.cfg.WebhookAddr = ":8080"
	}

	c.srv.Addr = c.cfg.WebhookAddr
	mux := http.NewServeMux()
	mux.HandleFunc(c.cfg.WebhookEndpoint, c.handleWebhook)
	c.srv.Handler = mux

	c.log.Info("gitlab webhook server started", "address", c.cfg.WebhookAddr, "endpoint", c.cfg.WebhookEndpoint)

	go func() {
		if err := c.srv.ListenAndServe(); err != nil && !errm.Is(err, http.ErrServerClosed) {
			c.log.Err(err, "gitlab webhook server failed to start")
		}
	}()

	go func() {
		<-ctx.Done()
		if err := c.srv.Shutdown(ctx); err != nil {
			c.log.Err(err, "gitlab webhook server failed to shutdown")
		}
	}()

	return nil
}

func (c *Client) handleWebhook(w http.ResponseWriter, r *http.Request) {
	// Проверяем токен из заголовка, чтобы убедиться, что запрос пришел от GitLab
	if r.Header.Get("X-Gitlab-Token") != c.cfg.WebhookSecret {
		c.log.Warn("received webhook with invalid token", "token", r.Header.Get("X-Gitlab-Token"))
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var payload WebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		c.log.Err(err, "failed to decode webhook payload")
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// Мы хотим реагировать только на открытие, переоткрытие или обновление MR
	// Также проверяем, что автор события - не наш бот, чтобы избежать зацикливания
	isRelevantAction := payload.ObjectAttributes.Action == "open" ||
		payload.ObjectAttributes.Action == "reopen" ||
		payload.ObjectAttributes.Action == "update"
	isNotBot := payload.User.Username != c.cfg.BotUsername

	log := c.log.WithFields(
		"ID", payload.ObjectAttributes.IID,
		"projectID", payload.Project.ID,
		"action", payload.ObjectAttributes.Action,
		"user", payload.User.Username,
		"objectKind", payload.ObjectKind,
	)

	if payload.ObjectKind == "merge_request" && isRelevantAction && isNotBot {
		log.Debug("received merge request webhook", "title", payload.ObjectAttributes.Title)
		// Add safety check for valid project ID
		if payload.Project.ID == 0 {
			c.log.Warn("received merge request webhook with invalid project ID", "MRID", payload.ObjectAttributes.IID)
			return
		}
		go c.processMR(payload.Project.ID, payload.ObjectAttributes.IID)

		return
	}

	log.Debug("received webhook")
}

// processMR - основная логика обработки Merge Request
func (c *Client) processMR(projectID, mrIID int) {

	mr, resp, err := c.cli.MergeRequests.GetMergeRequest(projectID, mrIID, nil)
	if err != nil {
		c.log.Err(err, "failed to get merge request", "projectID", projectID, "mrIID", mrIID)
		return
	}

	if resp.StatusCode != http.StatusOK {
		c.log.Error("got non-200 status code for merge request", "status", resp.StatusCode, "projectID", projectID, "mrIID", mrIID)
		return
	}

	log := c.log.WithFields(
		"last_commit_sha", mr.SHA[:8],
		"source_branch", mr.SourceBranch,
		"targetBranch", mr.TargetBranch,
		"author", mr.Author.Username,
	)

	if !slices.ContainsFunc(mr.Reviewers, func(r *gitlab.BasicUser) bool {
		return r.Username == c.cfg.BotUsername
	}) {
		log.Warn("skipping MR without bot as reviewer", "title", mr.Title)
		return
	}

	// Проверяем, обрабатывали ли мы уже этот конкретный коммит
	if c.isMRAlreadyProcessed(projectID, mrIID, mr.SHA) {
		log.Debug("MR already processed for this commit")
		return
	}

	log.Infof("processing MR: %s", mr.Title)

	// Добавляем задержку, чтобы GitLab успел обработать коммиты после пуша
	time.Sleep(5 * time.Second)

	// 1. Получаем изменения в MR с поддержкой пагинации
	diffs, err := c.getAllMergeRequestDiffs(projectID, mrIID, log)
	if err != nil {
		log.Err(err, "failed to get merge request changes")
		return
	}

	// 2. Определяем новые и изменившиеся файлы для анализа
	newAndChangedDiffs := c.getNewAndChangedFiles(projectID, mrIID, mr.SHA, diffs, log)

	// Фильтруем только код и конфигурационные файлы
	var codeFilesToReview []*gitlab.MergeRequestDiff
	var fullDiff strings.Builder

	for _, change := range newAndChangedDiffs {
		if change.DeletedFile {
			continue
		}
		if !isCodeFile(change.NewPath) {
			continue
		}
		if len(change.Diff) == 0 || len(change.Diff) > 10000 {
			continue
		}
		log.Debug("adding file to review", "file", change.NewPath)
		codeFilesToReview = append(codeFilesToReview, change)
		fullDiff.WriteString(fmt.Sprintf("--- a/%s\n+++ b/%s\n", change.OldPath, change.NewPath))
		fullDiff.WriteString(change.Diff)
		fullDiff.WriteString("\n\n")
	}

	// Отмечаем MR как обработанный для данного коммита
	c.markMRAsProcessed(projectID, mrIID, mr.SHA)

	if len(codeFilesToReview) == 0 {
		log.Info("no new code changes to analyze")
		return
	}

	log.Info("found changes to analyze", "new_or_changed_files", len(codeFilesToReview))

	// 3. Генерируем общее описание для MR (только при первом анализе или значительных изменениях)
	shouldUpdateDescription := !c.hasBotAlreadyCommented(projectID, mrIID, log) || len(codeFilesToReview) >= 3
	if shouldUpdateDescription && fullDiff.Len() > 0 {
		go c.generateAndPostMRDescription(projectID, mrIID, fullDiff.String(), log)
	}

	// 4. Анализируем новые и изменившиеся файлы
	go c.reviewCodeChanges(projectID, mrIID, codeFilesToReview, mr.SHA, log)
}

// generateAndPostMRDescription генерирует описание MR и обновляет его
func (c *Client) generateAndPostMRDescription(projectID, mrIID int, fullDiff string, log logze.Logger) {
	ctx := context.Background()

	// Call agent to generate description with retry logic
	description, err := c.generateDescriptionWithRetry(ctx, fullDiff, log)
	if err != nil {
		log.Err(err, "failed to generate description")
		return
	}

	if description == "" {
		log.Warn("agent did not generate description")
		return
	}

	// Получаем текущий MR, чтобы не затереть существующее описание
	mr, _, err := c.cli.MergeRequests.GetMergeRequest(projectID, mrIID, nil)
	if err != nil {
		log.Err(err, "failed to get merge request before updating description")
		return
	}

	// Формируем новое описание с проверкой на существующий автоматический блок
	newDescription := c.updateDescriptionWithAISection(mr.Description, description, log)

	updateOpts := &gitlab.UpdateMergeRequestOptions{
		Description: &newDescription,
	}
	_, _, err = c.cli.MergeRequests.UpdateMergeRequest(projectID, mrIID, updateOpts)
	if err != nil {
		log.Err(err, "failed to update description")
	} else {
		log.Info("successfully updated description")
	}
}

// reviewCodeChanges анализирует каждый файл и оставляет комментарии
func (c *Client) reviewCodeChanges(projectID int, mrIID int, changes []*gitlab.MergeRequestDiff, commitSHA string, log logze.Logger) {
	ctx := context.Background()
	for _, change := range changes {

		// Call agent to review code changes with retry logic
		reviewComment, err := c.reviewCodeWithRetry(ctx, change.NewPath, change.Diff, log)
		if err != nil {
			log.Err(err, "failed to review code for file", "file", change.NewPath)
			continue
		}

		// Если agent ответил "OK" (или что-то похожее) или ничего не вернул, пропускаем
		if reviewComment == "" || strings.HasPrefix(strings.TrimSpace(reviewComment), "OK") {
			log.Debug("no issues found for file", "file", change.NewPath)
			// Отмечаем файл как проанализированный, даже если комментарий не создается
			fileHash := getFileHash(change.Diff)
			c.markFileAsReviewed(projectID, mrIID, commitSHA, change.NewPath, fileHash)
			continue
		}

		// Форматируем комментарий
		fullCommentBody := fmt.Sprintf("### 🤖 Ревью для файла `%s`\n\n%s", change.NewPath, reviewComment)

		// Создаем обсуждение (комментарий) в MR
		discussionOpts := &gitlab.CreateMergeRequestDiscussionOptions{
			Body: &fullCommentBody,
		}
		_, _, err = c.cli.Discussions.CreateMergeRequestDiscussion(projectID, mrIID, discussionOpts)
		if err != nil {
			log.Err(err, "failed to create comment for file", "file", change.NewPath)
		} else {
			log.Info("successfully added comment for file", "file", change.NewPath)
			// Отмечаем файл как проанализированный
			fileHash := getFileHash(change.Diff)
			c.markFileAsReviewed(projectID, mrIID, commitSHA, change.NewPath, fileHash)
		}
	}
}

// getAllMergeRequestDiffs fetches all merge request diffs with pagination support
func (c *Client) getAllMergeRequestDiffs(projectID, mrIID int, log logze.Logger) ([]*gitlab.MergeRequestDiff, error) {
	var allDiffs []*gitlab.MergeRequestDiff
	page := 1

	for {
		opts := &gitlab.ListMergeRequestDiffsOptions{
			ListOptions: gitlab.ListOptions{
				Page: page,
			},
		}

		diffs, resp, err := c.cli.MergeRequests.ListMergeRequestDiffs(projectID, mrIID, opts)
		if err != nil {
			return nil, errm.Wrap(err, "failed to list merge request diffs")
		}

		if resp.StatusCode != http.StatusOK {
			return nil, errm.New(fmt.Sprintf("got non-200 status code for merge request changes: %d", resp.StatusCode))
		}

		allDiffs = append(allDiffs, diffs...)

		log.Debug("fetched merge request diffs page",
			"page", page,
			"diffsInPage", len(diffs),
			"totalSoFar", len(allDiffs),
		)

		// Проверяем, есть ли еще страницы
		if resp.NextPage == 0 {
			break
		}

		page = resp.NextPage
	}

	log.Debug("successfully fetched all merge request diffs",
		"totalDiffs", len(allDiffs),
		"totalPages", page,
	)

	return allDiffs, nil
}

// updateDescriptionWithAISection updates the MR description by replacing or adding the AI-generated section
func (*Client) updateDescriptionWithAISection(currentDescription, newAIDescription string, log logze.Logger) string {
	const (
		startMarker = "<!-- ai-desc-start -->"
		endMarker   = "<!-- ai-desc-end -->"
	)

	// Формируем новый блок с AI описанием
	aiSection := fmt.Sprintf("%s\n### 🤖 Описание изменений\n\n%s\n%s", startMarker, newAIDescription, endMarker)

	// Ищем существующий AI блок
	startIndex := strings.Index(currentDescription, startMarker)
	if startIndex == -1 {
		// Если AI блока нет, добавляем в начало
		if currentDescription == "" {
			return aiSection
		}
		return fmt.Sprintf("%s\n\n---\n\n%s", aiSection, currentDescription)
	}

	// Ищем конец AI блока
	endIndex := strings.Index(currentDescription, endMarker)
	if endIndex == -1 {
		// Если начальный маркер есть, а конечного нет - заменяем от начального маркера до конца
		log.Warn("found ai-desc-start marker but no ai-desc-end marker, replacing from start marker")
		return fmt.Sprintf("%s\n\n---\n\n%s", aiSection, currentDescription[startIndex:])
	}

	// Заменяем существующий AI блок новым
	endIndex += len(endMarker)
	beforeAI := currentDescription[:startIndex]
	afterAI := currentDescription[endIndex:]

	// Убираем лишние разделители если они есть
	beforeAI = strings.TrimRight(beforeAI, "\n\r \t")
	afterAI = strings.TrimLeft(afterAI, "\n\r \t")

	switch {
	case beforeAI == "" && afterAI == "":
		return aiSection
	case beforeAI == "":
		return fmt.Sprintf("%s\n\n---\n\n%s", aiSection, afterAI)
	case afterAI == "":
		return fmt.Sprintf("%s\n\n---\n\n%s", beforeAI, aiSection)
	default:
		return fmt.Sprintf("%s\n\n---\n\n%s\n\n---\n\n%s", beforeAI, aiSection, afterAI)
	}
}

// generateDescriptionWithRetry calls agent.GenerateDescription with retry logic for 429 errors
func (c *Client) generateDescriptionWithRetry(ctx context.Context, fullDiff string, log logze.Logger) (string, error) {
	description, err := c.agent.GenerateDescription(ctx, fullDiff)
	if err != nil && errm.Is(err, agent.ErrLimitExceeded) {
		log.Warn("got 429 error from agent, waiting 1 minute before retry")
		time.Sleep(c.cfg.IntervalToWaitLimits)

		// Retry once more
		description, err = c.agent.GenerateDescription(ctx, fullDiff)
		if err != nil {
			return "", errm.Wrap(err, "failed to generate description after retry")
		}
	}
	return description, err
}

// reviewCodeWithRetry calls agent.ReviewCode with retry logic for 429 errors
func (c *Client) reviewCodeWithRetry(ctx context.Context, filePath, diff string, log logze.Logger) (string, error) {
	reviewComment, err := c.agent.ReviewCode(ctx, filePath, diff)
	if err != nil && errm.Is(err, agent.ErrLimitExceeded) {
		log.Warn("got 429 error from agent, waiting 1 minute before retry", "file", filePath)
		time.Sleep(c.cfg.IntervalToWaitLimits)

		// Retry once more
		reviewComment, err = c.agent.ReviewCode(ctx, filePath, diff)
		if err != nil {
			return "", errm.Wrap(err, "failed to review code after retry")
		}
	}
	return reviewComment, err
}

// getMRKey создает уникальный ключ для MR на основе проекта, ID и последнего коммита
func getMRKey(projectID, mrIID int, lastCommitSHA string) string {
	return fmt.Sprintf("%d:%d:%s", projectID, mrIID, lastCommitSHA)
}

// isMRAlreadyProcessed проверяет, был ли MR уже обработан для данного коммита
func (c *Client) isMRAlreadyProcessed(projectID, mrIID int, lastCommitSHA string) bool {
	key := getMRKey(projectID, mrIID, lastCommitSHA)

	c.processedMRsMu.RLock()
	defer c.processedMRsMu.RUnlock()
	_, exists := c.processedMRs[key]
	return exists
}

// markMRAsProcessed отмечает MR как обработанный для данного коммита
func (c *Client) markMRAsProcessed(projectID, mrIID int, lastCommitSHA string) {
	key := getMRKey(projectID, mrIID, lastCommitSHA)
	c.processedMRsMu.Lock()
	defer c.processedMRsMu.Unlock()
	c.processedMRs[key] = true
}

// hasBotAlreadyCommented проверяет, есть ли уже комментарии бота в MR
func (c *Client) hasBotAlreadyCommented(projectID, mrIID int, log logze.Logger) bool {
	// Проверяем описание MR на наличие AI-секции
	mr, _, err := c.cli.MergeRequests.GetMergeRequest(projectID, mrIID, nil)
	if err != nil {
		log.Err(err, "failed to get merge request for duplication check")
		return false
	}

	// Проверяем наличие AI-секции в описании
	if strings.Contains(mr.Description, "<!-- ai-desc-start -->") {
		log.Debug("found existing AI description section")
		return true
	}

	// Проверяем комментарии/обсуждения от бота
	discussions, _, err := c.cli.Discussions.ListMergeRequestDiscussions(projectID, mrIID, nil)
	if err != nil {
		log.Err(err, "failed to get discussions for duplication check")
		return false
	}

	for _, discussion := range discussions {
		for _, note := range discussion.Notes {
			if note.Author.Username == c.cfg.BotUsername && strings.Contains(note.Body, "🤖 Ревью для файла") {
				log.Debug("found existing bot review comment")
				return true
			}
		}
	}

	return false
}

// getFileHash создает хеш для содержимого файла на основе diff
func getFileHash(diff string) string {
	if diff == "" {
		return ""
	}
	// Используем простой хеш на основе длины и первых символов diff'а
	// Это достаточно для определения изменений в файле
	if len(diff) > 100 {
		return fmt.Sprintf("%d:%s", len(diff), diff[:100])
	}
	return fmt.Sprintf("%d:%s", len(diff), diff)
}

// markFileAsReviewed отмечает файл как проанализированный для данного коммита
func (c *Client) markFileAsReviewed(projectID, mrIID int, commitSHA, filePath, fileHash string) {
	key := getMRKey(projectID, mrIID, commitSHA)

	c.reviewedFilesMu.Lock()
	defer c.reviewedFilesMu.Unlock()

	if c.reviewedFiles[key] == nil {
		c.reviewedFiles[key] = make(map[string]string)
	}

	c.reviewedFiles[key][filePath] = fileHash
}

// getNewAndChangedFiles определяет, какие файлы новые или изменились с последнего анализа
func (c *Client) getNewAndChangedFiles(projectID, mrIID int, currentCommitSHA string, diffs []*gitlab.MergeRequestDiff, log logze.Logger) []*gitlab.MergeRequestDiff {
	// Получаем список ранее проанализированных файлов для любого коммита этого MR
	var previouslyReviewedFiles map[string]string

	// Ищем предыдущие коммиты этого MR
	c.reviewedFilesMu.RLock()
	for keyStr, reviewedFiles := range c.reviewedFiles {
		if strings.HasPrefix(keyStr, fmt.Sprintf("%d:%d:", projectID, mrIID)) && !strings.HasSuffix(keyStr, currentCommitSHA) {
			if previouslyReviewedFiles == nil {
				previouslyReviewedFiles = make(map[string]string)
			}
			// Объединяем файлы из всех предыдущих коммитов
			for filePath, fileHash := range reviewedFiles {
				previouslyReviewedFiles[filePath] = fileHash
			}
		}
	}
	c.reviewedFilesMu.RUnlock()

	if previouslyReviewedFiles == nil {
		log.Debug("no previously reviewed files found, processing all diffs")
		return diffs
	}

	var newAndChangedFiles []*gitlab.MergeRequestDiff
	for _, diff := range diffs {
		fileHash := getFileHash(diff.Diff)
		filePath := diff.NewPath

		if previousHash, exists := previouslyReviewedFiles[filePath]; !exists {
			newAndChangedFiles = append(newAndChangedFiles, diff)
		} else if previousHash != fileHash {
			newAndChangedFiles = append(newAndChangedFiles, diff)
		}
	}

	log.Debug("identified files for review",
		"total_files", len(diffs),
		"new_or_changed_files", len(newAndChangedFiles),
		"previously_reviewed_files", len(previouslyReviewedFiles))

	return newAndChangedFiles
}
