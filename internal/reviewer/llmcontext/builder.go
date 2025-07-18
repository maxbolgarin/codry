package llmcontext

import (
	"context"
	"time"

	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/codry/internal/reviewer/astparser"

	"github.com/maxbolgarin/errm"
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

	repoDataProvider *repoDataProvider
}

// NewBuilder creates a new context bundle builder
func NewBuilder(provider interfaces.CodeProvider, isVerbose bool) *Builder {
	return &Builder{
		provider:         provider,
		contextFinder:    astparser.NewContextFinder(provider),
		symbolAnalyzer:   astparser.NewAnalyzer(provider),
		diffParser:       astparser.NewDiffParser(),
		astParser:        astparser.NewParser(),
		log:              logze.With("component", "context_bundle_builder"),
		isVerbose:        isVerbose,
		repoDataProvider: newRepoDataProvider(provider, isVerbose),
	}
}

// BuildContext builds a comprehensive context bundle for LLM analysis
func (cbb *Builder) BuildContext(ctx context.Context, projectID string, mrIID int) (*ContextBundle, error) {
	cbb.log.DebugIf(cbb.isVerbose, "loading repository data")

	err := cbb.repoDataProvider.loadData(ctx, projectID, mrIID)
	if err != nil {
		return nil, errm.Wrap(err, "failed to load repository data")
	}

	cbb.log.DebugIf(cbb.isVerbose, "loaded all data for context gathering")

	mrContext, err := gatherMRContext(projectID, cbb.repoDataProvider)
	if err != nil {
		return nil, errm.Wrap(err, "failed to get MR context")
	}

	cbb.log.DebugIf(cbb.isVerbose, "gathered MR context")

	contextRequest := astparser.ContextRequest{
		ProjectID:    projectID,
		MergeRequest: cbb.repoDataProvider.mr,
		FileDiffs:    cbb.repoDataProvider.diffs,
		RepoDataHead: cbb.repoDataProvider.repoDataHead,
		RepoDataBase: cbb.repoDataProvider.repoDataBase,
	}

	// Gather basic context using ContextFinder
	filesContext, err := cbb.contextFinder.GatherFilesContext(ctx, contextRequest)
	if err != nil {
		return nil, errm.Wrap(err, "failed to gather basic context")
	}

	cbb.log.DebugIf(cbb.isVerbose, "gathered basic context")

	bundle := &ContextBundle{
		Files:     filesContext,
		MRContext: mrContext,
	}

	cbb.log.DebugIf(cbb.isVerbose, "built context bundle")
	time.Sleep(time.Second)

	return bundle, nil
}
