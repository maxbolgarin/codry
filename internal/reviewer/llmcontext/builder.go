package llmcontext

import (
	"context"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/codry/internal/reviewer/astparser"
	"github.com/maxbolgarin/erro"

	"github.com/maxbolgarin/logze/v2"
)

// Builder builds comprehensive context bundles for LLM analysis
type Builder struct {
	provider       interfaces.CodeProvider
	contextFinder  *astparser.ContextManager
	symbolAnalyzer *astparser.Analyzer
	diffParser     *astparser.DiffParser
	astParser      *astparser.ASTParser
	log            logze.Logger
	isVerbose      bool

	cfg Filter

	repoDataProvider *repoDataProvider
}

// NewBuilder creates a new context bundle builder
func NewBuilder(provider interfaces.CodeProvider, cfg Filter, isVerbose bool) *Builder {
	return &Builder{
		provider:         provider,
		contextFinder:    astparser.NewContextFinder(provider),
		symbolAnalyzer:   astparser.NewAnalyzer(provider),
		diffParser:       astparser.NewDiffParser(),
		astParser:        astparser.NewParser(),
		log:              logze.With("component", "context_bundle_builder"),
		isVerbose:        isVerbose,
		cfg:              cfg,
		repoDataProvider: newRepoDataProvider(provider, isVerbose),
	}
}

// BuildContext builds a comprehensive context bundle for LLM analysis
func (cbb *Builder) BuildContext(ctx context.Context, projectID string, mrIID int) (*ContextBundle, error) {
	cbb.log.DebugIf(cbb.isVerbose, "loading repository data")

	err := cbb.repoDataProvider.loadMRData(ctx, projectID, mrIID)
	if err != nil {
		return nil, erro.Wrap(err, "failed to load repository data")
	}
	filesForReview, totalDiffLength := cbb.filterFilesForReview()
	if len(filesForReview) == 0 {
		return nil, erro.New("no files to review")
	}

	err = cbb.repoDataProvider.loadAllData(ctx, projectID, mrIID)
	if err != nil {
		return nil, erro.Wrap(err, "failed to load repository data")
	}
	cbb.log.DebugIf(cbb.isVerbose, "loaded all data for context gathering")

	mrContext, err := gatherMRContext(projectID, cbb.repoDataProvider)
	if err != nil {
		return nil, erro.Wrap(err, "failed to get MR context")
	}
	cbb.log.DebugIf(cbb.isVerbose, "gathered MR context")

	contextRequest := astparser.ContextRequest{
		ProjectID:      projectID,
		MergeRequest:   cbb.repoDataProvider.mr,
		FilesForReview: filesForReview,
		RepoDataHead:   cbb.repoDataProvider.repoDataHead,
		RepoDataBase:   cbb.repoDataProvider.repoDataBase,
	}

	// Gather basic context using ContextFinder
	filesContext, err := cbb.contextFinder.GatherFilesContext(ctx, contextRequest)
	if err != nil {
		return nil, erro.Wrap(err, "failed to gather basic context")
	}

	cbb.log.DebugIf(cbb.isVerbose, "gathered basic context")

	filesForReviewContext := make([]*FileContext, 0, len(filesContext))
	for i, fileContext := range filesContext {
		filesForReviewContext = append(filesForReviewContext, &FileContext{
			Diff:    filesForReview[i],
			Context: fileContext,
		})
	}

	bundle := &ContextBundle{
		FilesForReview:  filesForReviewContext,
		MR:              mrContext,
		TotalDiffLength: totalDiffLength,
	}

	cbb.log.DebugIf(cbb.isVerbose, "built context bundle")

	return bundle, nil
}

func (cbb *Builder) filterFilesForReview() ([]*model.FileDiff, int64) {
	var filtered []*model.FileDiff

	var totalDiffLength int64

	for _, file := range cbb.repoDataProvider.diffs {
		if file.IsDeleted || file.IsBinary {
			cbb.debugVerbose("skipping deleted or binary file", "file", file.NewPath)
			continue
		}

		if len(file.Diff) == 0 {
			cbb.debugVerbose("skipping empty file", "file", file.NewPath)
			continue
		}

		if len(file.Diff) > cbb.cfg.MaxFileSizeTokens {
			cbb.debugVerbose("skipping due to size", "file", file.NewPath, "size", len(file.Diff), "max_size", cbb.cfg.MaxFileSizeTokens)
			continue
		}

		if cbb.cfg.isExcludedPath(file.NewPath) {
			cbb.debugVerbose("skipping excluded", "file", file.NewPath)
			continue
		}

		if !cbb.cfg.isAllowedExtension(file.NewPath) {
			cbb.debugVerbose("skipping non-code", "file", file.NewPath)
			continue
		}

		cbb.log.DebugIf(cbb.isVerbose, "adding to review", "file", file.NewPath)
		filtered = append(filtered, file)

		// Count diff string total size
		totalDiffLength += int64(len(file.Diff))
		totalDiffLength += int64(len(file.OldPath))
		totalDiffLength += int64(len(file.NewPath))

		// Limit number of files per MR
		if len(filtered) >= cbb.cfg.MaxFiles {
			cbb.log.Warn("reached maximum files limit", "limit", cbb.cfg.MaxFiles)
			break
		}
	}

	if len(filtered) == 0 {
		cbb.logFlow("no files to review after filtering")
		return nil, 0
	}

	cbb.logFlow("found files to review",
		"total_files", len(filtered),
		"diff_length", totalDiffLength,
	)

	return filtered, totalDiffLength
}

func (cbb *Builder) debugVerbose(msg string, fields ...any) {
	cbb.log.DebugIf(cbb.isVerbose, msg, fields...)
}

func (cbb *Builder) logFlow(msg string, fields ...any) {
	if cbb.isVerbose {
		cbb.log.Info(msg, fields...)
	} else {
		cbb.log.Debug(msg, fields...)
	}
}
