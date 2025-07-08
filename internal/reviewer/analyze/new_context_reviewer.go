package analyze

// import (
// 	"context"

// 	"github.com/maxbolgarin/codry/internal/agent"
// 	"github.com/maxbolgarin/codry/internal/agent/prompts"
// 	"github.com/maxbolgarin/codry/internal/model"
// 	"github.com/maxbolgarin/codry/internal/model/interfaces"
// 	"github.com/maxbolgarin/errm"
// 	"github.com/maxbolgarin/logze/v2"
// )

// // NewSemanticContextReviewer creates a new reviewer with enhanced semantic context
// func NewSemanticContextReviewer(cfg Config, provider interfaces.CodeProvider, ag *agent.Agent) *SemanticContextReviewer {
// 	return &SemanticContextReviewer{
// 		cfg:                    cfg,
// 		provider:               provider,
// 		agent:                  ag,
// 		enhancedContextBuilder: NewEnhancedContextBuilder(provider),
// 		parser:                 newDiffParser(),
// 		log:                    logze.With("component", "semantic-context-reviewer"),
// 	}
// }

// // SemanticContextReviewer performs code review with enhanced semantic context
// type SemanticContextReviewer struct {
// 	cfg                    Config
// 	provider               interfaces.CodeProvider
// 	agent                  *agent.Agent
// 	enhancedContextBuilder *EnhancedContextBuilder
// 	parser                 *diffParser
// 	log                    logze.Logger
// }

// // ReviewFileWithSemanticContext performs enhanced semantic code review
// func (scr *SemanticContextReviewer) ReviewFileWithSemanticContext(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff) (*model.FileReviewResult, error) {

// 	// Step 1: Build comprehensive targeted context
// 	targetedContext, err := scr.enhancedContextBuilder.BuildTargetedContext(ctx, request, fileDiff)
// 	if err != nil {
// 		return nil, errm.Wrap(err, "failed to build targeted context")
// 	}

// 	// Step 2: Generate clean diff
// 	cleanDiff, err := scr.parser.GenerateCleanDiff(fileDiff.Diff)
// 	if err != nil {
// 		return nil, err
// 	}

// 	// Step 3: Convert to prompts context format
// 	promptsContext := scr.enhancedContextBuilder.ConvertToPromptsContext(targetedContext)
// 	promptsContext.FilePath = fileDiff.NewPath
// 	promptsContext.CleanDiff = cleanDiff

// 	// Step 4: Enhance the prompts context with our semantic insights
// 	scr.enhancePromptsContext(promptsContext, targetedContext)

// 	// Step 5: Perform AI review with enhanced context
// 	reviewResult, err := scr.agent.ReviewCodeWithContext(ctx, fileDiff.NewPath, promptsContext)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return reviewResult, nil
// }

// // enhancePromptsContext adds our semantic insights to the prompts context
// func (scr *SemanticContextReviewer) enhancePromptsContext(promptsCtx *prompts.EnhancedContext, targetedCtx *TargetedContext) {
// 	// Add business context information to related files
// 	for _, entityCtx := range targetedCtx.ChangedEntities {
// 		relatedFile := prompts.RelatedFile{
// 			Path:         entityCtx.Entity.FilePath,
// 			Relationship: string(entityCtx.ChangeType) + "_entity",
// 			Snippet:      scr.buildEntityContextSnippet(entityCtx),
// 		}
// 		promptsCtx.RelatedFiles = append(promptsCtx.RelatedFiles, relatedFile)
// 	}

// 	// Note: Usage patterns are now generated directly in ConvertToPromptsContext
// 	// No need to add them here as they're already included

// 	// Add before/after context as related files for comparison
// 	for _, pair := range targetedCtx.BeforeAfterPairs {
// 		if pair.BeforeCode != "" {
// 			relatedFile := prompts.RelatedFile{
// 				Path:         "BEFORE_" + pair.EntityName,
// 				Relationship: "before_state",
// 				Snippet:      pair.BeforeCode,
// 			}
// 			promptsCtx.RelatedFiles = append(promptsCtx.RelatedFiles, relatedFile)
// 		}
// 	}

// 	// Add related code snippets
// 	for _, snippet := range targetedCtx.RelatedCode {
// 		relatedFile := prompts.RelatedFile{
// 			Path:         snippet.FilePath,
// 			Relationship: snippet.Relationship,
// 			Snippet:      snippet.CodeSnippet,
// 		}
// 		promptsCtx.RelatedFiles = append(promptsCtx.RelatedFiles, relatedFile)
// 	}

// 	// Enhance semantic changes with our analysis
// 	for _, entityCtx := range targetedCtx.ChangedEntities {
// 		semanticChange := prompts.SemanticChange{
// 			Type:        scr.mapEntityTypeToSemanticType(entityCtx.Entity.Type),
// 			Impact:      entityCtx.RiskLevel,
// 			Description: scr.buildSemanticDescription(entityCtx),
// 			Lines:       []int{entityCtx.Entity.StartLine, entityCtx.Entity.EndLine},
// 			Context:     entityCtx.BusinessRelevance,
// 		}
// 		promptsCtx.SemanticChanges = append(promptsCtx.SemanticChanges, semanticChange)
// 	}
// }

// // buildEntityContextSnippet creates a context snippet for an entity
// func (scr *SemanticContextReviewer) buildEntityContextSnippet(entityCtx EntityContext) string {
// 	snippet := "ENTITY CONTEXT:\n"
// 	snippet += "Type: " + string(entityCtx.Entity.Type) + "\n"
// 	snippet += "Name: " + entityCtx.Entity.Name + "\n"
// 	snippet += "Business Area: " + entityCtx.Entity.BusinessArea + "\n"
// 	snippet += "Security Level: " + entityCtx.Entity.SecurityLevel + "\n"
// 	snippet += "Risk Level: " + entityCtx.RiskLevel + "\n"

// 	if len(entityCtx.Dependencies) > 0 {
// 		snippet += "Dependencies: "
// 		for i, dep := range entityCtx.Dependencies {
// 			if i > 0 {
// 				snippet += ", "
// 			}
// 			snippet += dep.Relationship.Target
// 		}
// 		snippet += "\n"
// 	}

// 	if len(entityCtx.Dependents) > 0 {
// 		snippet += "Dependents: "
// 		for i, dep := range entityCtx.Dependents {
// 			if i > 0 {
// 				snippet += ", "
// 			}
// 			snippet += dep.Relationship.Target
// 		}
// 		snippet += "\n"
// 	}

// 	snippet += "\nCODE:\n" + entityCtx.AfterCode

// 	return snippet
// }

// // mapEntityTypeToSemanticType maps our entity types to semantic change types
// func (scr *SemanticContextReviewer) mapEntityTypeToSemanticType(entityType EntityType) string {
// 	switch entityType {
// 	case EntityTypeFunction, EntityTypeMethod:
// 		return "business_logic"
// 	case EntityTypeType, EntityTypeStruct, EntityTypeInterface:
// 		return "api_contract"
// 	case EntityTypeConst, EntityTypeVar:
// 		return "configuration"
// 	default:
// 		return "other"
// 	}
// }

// // buildSemanticDescription creates a semantic description for an entity change
// func (scr *SemanticContextReviewer) buildSemanticDescription(entityCtx EntityContext) string {
// 	desc := string(entityCtx.ChangeType) + " " + string(entityCtx.Entity.Type) + " '" + entityCtx.Entity.Name + "'"

// 	if entityCtx.Entity.BusinessArea != "general" {
// 		desc += " in " + entityCtx.Entity.BusinessArea + " domain"
// 	}

// 	if entityCtx.Entity.SecurityLevel == "high" {
// 		desc += " (security-sensitive)"
// 	}

// 	if len(entityCtx.Dependents) > 0 {
// 		desc += " with " + string(len(entityCtx.Dependents)) + " dependents"
// 	}

// 	return desc
// }

// // performBasicReview fallback to basic review if semantic analysis fails
// func (scr *SemanticContextReviewer) performBasicReview(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff) (*model.FileReviewResult, error) {
// 	// Generate clean diff
// 	cleanDiff, err := scr.parser.GenerateCleanDiff(fileDiff.Diff)
// 	if err != nil {
// 		return nil, err
// 	}

// 	// Get file content if available
// 	var fileContent string
// 	if !fileDiff.IsNew && !fileDiff.IsDeleted {
// 		content, err := scr.provider.GetFileContent(ctx, request.ProjectID, fileDiff.NewPath, request.MergeRequest.TargetBranch)
// 		if err == nil {
// 			fileContent = content
// 		}
// 	}

// 	// Create basic enhanced context
// 	basicContext := &prompts.EnhancedContext{
// 		FilePath:    fileDiff.NewPath,
// 		FileContent: fileContent,
// 		CleanDiff:   cleanDiff,
// 	}

// 	return scr.agent.ReviewCodeWithContext(ctx, fileDiff.NewPath, basicContext)
// }
