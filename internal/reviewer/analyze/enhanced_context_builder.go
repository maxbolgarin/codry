package analyze

import (
	"context"
	"fmt"
	"strings"

	"github.com/maxbolgarin/codry/internal/agent/prompts"
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/codry/internal/model/interfaces"
	"github.com/maxbolgarin/logze/v2"
)

// EnhancedContextBuilder builds sophisticated, targeted context for AI code review
type EnhancedContextBuilder struct {
	provider         interfaces.CodeProvider
	semanticAnalyzer *SemanticAnalyzer
	styleAnalyzer    *ProjectStyleAnalyzer
	dependencyMapper *DependencyMapper
	log              logze.Logger
}

// NewEnhancedContextBuilder creates a new enhanced context builder
func NewEnhancedContextBuilder(provider interfaces.CodeProvider) *EnhancedContextBuilder {
	return &EnhancedContextBuilder{
		provider:         provider,
		semanticAnalyzer: NewSemanticAnalyzer(provider),
		styleAnalyzer:    NewProjectStyleAnalyzer(provider),
		dependencyMapper: NewDependencyMapper(provider),
		log:              logze.With("component", "enhanced-context-builder"),
	}
}

// TargetedContext represents focused, semantic context for code review
type TargetedContext struct {
	// Core change information
	ChangedEntities  []EntityContext         `json:"changed_entities"`  // entities that were changed
	DependencyGraph  *DependencyGraph        `json:"dependency_graph"`  // semantic relationships
	ProjectStyle     *ProjectStyleInfo       `json:"project_style"`     // project-specific patterns
	SemanticAnalysis *SemanticAnalysisResult `json:"semantic_analysis"` // semantic analysis results

	// Focused code snippets (not entire files)
	BeforeAfterPairs []BeforeAfterPair    `json:"before_after_pairs"` // before/after code for changed entities
	RelatedCode      []RelatedCodeSnippet `json:"related_code"`       // relevant code from dependencies/dependents

	// Contextual insights
	BusinessImpact       BusinessImpactInfo       `json:"business_impact"`       // business-level impact assessment
	ArchitecturalContext ArchitecturalContextInfo `json:"architectural_context"` // architectural context
	QualityContext       QualityContextInfo       `json:"quality_context"`       // code quality context
	SecurityContext      SecurityContextInfo      `json:"security_context"`      // security implications

	// AI guidance
	ReviewGuidance ReviewGuidanceInfo `json:"review_guidance"` // guidance for the AI reviewer
	FocusAreas     []FocusArea        `json:"focus_areas"`     // areas that need special attention
}

// EntityContext represents context for a specific changed entity
type EntityContext struct {
	Entity            *CodeEntity         `json:"entity"`             // the entity itself
	ChangeType        ChangeType          `json:"change_type"`        // how it was changed
	BeforeCode        string              `json:"before_code"`        // code before change
	AfterCode         string              `json:"after_code"`         // code after change
	Dependencies      []DependencyContext `json:"dependencies"`       // what it depends on
	Dependents        []DependentContext  `json:"dependents"`         // what depends on it
	BusinessRelevance string              `json:"business_relevance"` // business significance
	RiskLevel         string              `json:"risk_level"`         // risk level of the change
}

// DependencyContext represents a dependency with its context
type DependencyContext struct {
	Entity       *CodeEntity  `json:"entity"`       // the dependency entity
	Relationship Relationship `json:"relationship"` // relationship details
	CodeSnippet  string       `json:"code_snippet"` // relevant code showing the dependency
	IsExternal   bool         `json:"is_external"`  // whether it's external to the project
	IsChanged    bool         `json:"is_changed"`   // whether this dependency is also being changed
}

// DependentContext represents something that depends on the changed entity
type DependentContext struct {
	Entity       *CodeEntity  `json:"entity"`        // the dependent entity
	Relationship Relationship `json:"relationship"`  // relationship details
	CodeSnippet  string       `json:"code_snippet"`  // relevant code showing the dependency
	ImpactLevel  string       `json:"impact_level"`  // level of impact from the change
	BreakingRisk string       `json:"breaking_risk"` // risk of breaking this dependent
}

// BeforeAfterPair represents before and after code for comparison
type BeforeAfterPair struct {
	EntityName  string `json:"entity_name"`  // name of the entity
	EntityType  string `json:"entity_type"`  // type of entity
	BeforeCode  string `json:"before_code"`  // code before change
	AfterCode   string `json:"after_code"`   // code after change
	ChangeType  string `json:"change_type"`  // type of change
	LineNumbers []int  `json:"line_numbers"` // relevant line numbers
	Explanation string `json:"explanation"`  // explanation of the change
}

// RelatedCodeSnippet represents relevant code from related entities
type RelatedCodeSnippet struct {
	EntityName   string `json:"entity_name"`  // name of the related entity
	EntityType   string `json:"entity_type"`  // type of entity
	FilePath     string `json:"file_path"`    // file path
	CodeSnippet  string `json:"code_snippet"` // the code snippet
	Relationship string `json:"relationship"` // how it relates to the changed code
	Relevance    string `json:"relevance"`    // why it's relevant
	LineNumbers  []int  `json:"line_numbers"` // line numbers in the source file
}

// BusinessImpactInfo represents business-level impact assessment
type BusinessImpactInfo struct {
	Domain          string   `json:"domain"`           // business domain
	Criticality     string   `json:"criticality"`      // business criticality
	UserImpact      string   `json:"user_impact"`      // impact on users
	DataSensitivity string   `json:"data_sensitivity"` // data sensitivity level
	ComplianceAreas []string `json:"compliance_areas"` // compliance considerations
	RiskAreas       []string `json:"risk_areas"`       // risk areas to watch
	Stakeholders    []string `json:"stakeholders"`     // affected stakeholders
}

// ArchitecturalContextInfo represents architectural context
type ArchitecturalContextInfo struct {
	Layer            string   `json:"layer"`             // architectural layer
	Components       []string `json:"components"`        // affected components
	Boundaries       []string `json:"boundaries"`        // architectural boundaries
	Patterns         []string `json:"patterns"`          // architectural patterns
	DesignPrinciples []string `json:"design_principles"` // relevant design principles
	Constraints      []string `json:"constraints"`       // architectural constraints
}

// QualityContextInfo represents code quality context
type QualityContextInfo struct {
	ComplexityLevel       string   `json:"complexity_level"`       // complexity level
	TestabilityImpact     string   `json:"testability_impact"`     // impact on testability
	MaintainabilityImpact string   `json:"maintainability_impact"` // impact on maintainability
	PerformanceImpact     string   `json:"performance_impact"`     // potential performance impact
	QualityRisks          []string `json:"quality_risks"`          // quality risks
	BestPractices         []string `json:"best_practices"`         // applicable best practices
	AntiPatterns          []string `json:"anti_patterns"`          // anti-patterns to avoid
}

// SecurityContextInfo represents security implications
type SecurityContextInfo struct {
	SecurityLevel    string   `json:"security_level"`    // security sensitivity level
	ThreatAreas      []string `json:"threat_areas"`      // potential threat areas
	SecurityPatterns []string `json:"security_patterns"` // relevant security patterns
	ComplianceImpact string   `json:"compliance_impact"` // compliance implications
	SecurityRisks    []string `json:"security_risks"`    // security risks
	Mitigations      []string `json:"mitigations"`       // recommended mitigations
}

// ReviewGuidanceInfo provides guidance for the AI reviewer
type ReviewGuidanceInfo struct {
	PrimaryFocus    string   `json:"primary_focus"`    // what to focus on primarily
	SecondaryFocus  []string `json:"secondary_focus"`  // secondary areas of focus
	CommonIssues    []string `json:"common_issues"`    // common issues in this type of change
	ProjectSpecific []string `json:"project_specific"` // project-specific things to check
	BusinessContext string   `json:"business_context"` // business context for the review
	ReviewStrategy  string   `json:"review_strategy"`  // suggested review strategy
	IgnorePatterns  []string `json:"ignore_patterns"`  // patterns that are okay in this project
}

// FocusArea represents an area that needs special attention
type FocusArea struct {
	Name       string `json:"name"`       // name of the focus area
	Priority   string `json:"priority"`   // priority level (high, medium, low)
	Reason     string `json:"reason"`     // why this area needs focus
	Specifics  string `json:"specifics"`  // specific things to look for
	Examples   string `json:"examples"`   // examples of issues to look for
	Guidelines string `json:"guidelines"` // guidelines for reviewing this area
}

// BuildTargetedContext creates comprehensive, targeted context for code review
func (ecb *EnhancedContextBuilder) BuildTargetedContext(ctx context.Context, request model.ReviewRequest, fileDiff *model.FileDiff) (*TargetedContext, error) {
	log := ecb.log.WithFields("file", fileDiff.NewPath, "project", request.ProjectID)
	targetedCtx := &TargetedContext{}

	// Step 1: Perform semantic analysis to understand what changed
	semanticResult, err := ecb.semanticAnalyzer.AnalyzeChanges(ctx, request, fileDiff)
	if err != nil {
		log.Warn("failed semantic analysis", "error", err)
		semanticResult = &SemanticAnalysisResult{} // Use empty result as fallback
	}
	targetedCtx.SemanticAnalysis = semanticResult

	// Step 2: Analyze project style and conventions
	projectStyle, err := ecb.styleAnalyzer.AnalyzeProjectStyle(ctx, request, fileDiff.NewPath)
	if err != nil {
		log.Warn("failed project style analysis", "error", err)
		projectStyle = &ProjectStyleInfo{} // Use empty result as fallback
	}
	targetedCtx.ProjectStyle = projectStyle

	// Step 3: Map dependencies and relationships
	dependencyGraph, err := ecb.dependencyMapper.MapDependencies(ctx, request, semanticResult.ChangedEntities, fileDiff.NewPath)
	if err != nil {
		log.Warn("failed dependency mapping", "error", err)
		dependencyGraph = &DependencyGraph{} // Use empty result as fallback
	}
	targetedCtx.DependencyGraph = dependencyGraph

	// Step 4: Build entity contexts with rich information
	targetedCtx.ChangedEntities = ecb.buildEntityContexts(semanticResult.ChangedEntities, dependencyGraph)

	// Step 5: Create before/after pairs for easy comparison
	targetedCtx.BeforeAfterPairs = ecb.buildBeforeAfterPairs(semanticResult.ChangedEntities)

	// Step 6: Gather related code snippets (not entire files)
	targetedCtx.RelatedCode = ecb.buildRelatedCodeSnippets(ctx, request, dependencyGraph, fileDiff.NewPath)

	// Step 7: Build contextual insights
	targetedCtx.BusinessImpact = ecb.buildBusinessImpact(semanticResult.BusinessContext, semanticResult.ChangedEntities)
	targetedCtx.ArchitecturalContext = ecb.buildArchitecturalContext(semanticResult.ArchitecturalScope)
	targetedCtx.QualityContext = ecb.buildQualityContext(projectStyle, dependencyGraph, semanticResult.ChangedEntities)
	targetedCtx.SecurityContext = ecb.buildSecurityContext(projectStyle.SecurityPatterns, semanticResult.ChangedEntities)

	// Step 8: Generate review guidance for the AI
	targetedCtx.ReviewGuidance = ecb.buildReviewGuidance(targetedCtx)
	targetedCtx.FocusAreas = ecb.buildFocusAreas(targetedCtx)

	log.Debug("targeted context built successfully",
		"changed_entities", len(targetedCtx.ChangedEntities),
		"related_code_snippets", len(targetedCtx.RelatedCode),
		"focus_areas", len(targetedCtx.FocusAreas))

	return targetedCtx, nil
}

// buildEntityContexts creates rich context for each changed entity
func (ecb *EnhancedContextBuilder) buildEntityContexts(changedEntities []ChangedEntity, graph *DependencyGraph) []EntityContext {
	var contexts []EntityContext

	for _, entity := range changedEntities {
		entityID := fmt.Sprintf("%s.%s.%s", extractPackageFromPath(entity.Name), string(entity.Type), entity.Name)

		// Get entity from graph if available
		var codeEntity *CodeEntity
		if graphEntity, exists := graph.Entities[entityID]; exists {
			codeEntity = graphEntity
		} else {
			// Create a basic code entity
			codeEntity = &CodeEntity{
				ID:            entityID,
				Name:          entity.Name,
				Type:          entity.Type,
				IsExported:    entity.IsExported,
				Signature:     entity.Signature,
				CodeSnippet:   entity.AfterCode,
				BusinessArea:  inferBusinessAreaFromEntity(entity),
				SecurityLevel: inferSecurityLevelFromEntity(entity),
			}
		}

		// Build dependency contexts
		var dependencyContexts []DependencyContext
		if deps, exists := graph.Dependencies[entityID]; exists {
			for _, dep := range deps {
				dependencyContexts = append(dependencyContexts, DependencyContext{
					Relationship: dep,
					CodeSnippet:  dep.CodeSnippet,
					IsExternal:   dep.IsExternal,
				})
			}
		}

		// Build dependent contexts
		var dependentContexts []DependentContext
		if deps, exists := graph.Dependents[entityID]; exists {
			for _, dep := range deps {
				dependentContexts = append(dependentContexts, DependentContext{
					Relationship: dep,
					CodeSnippet:  dep.CodeSnippet,
					ImpactLevel:  calculateImpactLevel(dep),
					BreakingRisk: calculateBreakingRisk(entity, dep),
				})
			}
		}

		contexts = append(contexts, EntityContext{
			Entity:            codeEntity,
			ChangeType:        entity.ChangeType,
			BeforeCode:        entity.BeforeCode,
			AfterCode:         entity.AfterCode,
			Dependencies:      dependencyContexts,
			Dependents:        dependentContexts,
			BusinessRelevance: calculateBusinessRelevance(entity),
			RiskLevel:         calculateRiskLevel(entity, dependentContexts),
		})
	}

	return contexts
}

// buildBeforeAfterPairs creates before/after code pairs for comparison
func (ecb *EnhancedContextBuilder) buildBeforeAfterPairs(changedEntities []ChangedEntity) []BeforeAfterPair {
	var pairs []BeforeAfterPair

	for _, entity := range changedEntities {
		if entity.BeforeCode != "" || entity.AfterCode != "" {
			pair := BeforeAfterPair{
				EntityName:  entity.Name,
				EntityType:  string(entity.Type),
				BeforeCode:  entity.BeforeCode,
				AfterCode:   entity.AfterCode,
				ChangeType:  string(entity.ChangeType),
				LineNumbers: []int{entity.StartLine, entity.EndLine},
				Explanation: generateChangeExplanation(entity),
			}
			pairs = append(pairs, pair)
		}
	}

	return pairs
}

// buildRelatedCodeSnippets gathers relevant code snippets from related entities
func (ecb *EnhancedContextBuilder) buildRelatedCodeSnippets(ctx context.Context, request model.ReviewRequest, graph *DependencyGraph, filePath string) []RelatedCodeSnippet {
	var snippets []RelatedCodeSnippet

	// Collect snippets from high-strength relationships
	for entityID, relationships := range graph.Dependencies {
		for _, rel := range relationships {
			if rel.Strength > 0.7 && rel.CodeSnippet != "" { // Only high-strength relationships
				snippet := RelatedCodeSnippet{
					EntityName:   rel.Target,
					EntityType:   string(rel.Type),
					FilePath:     rel.FilePath,
					CodeSnippet:  rel.CodeSnippet,
					Relationship: string(rel.Type),
					Relevance:    fmt.Sprintf("Used by %s", entityID),
					LineNumbers:  []int{rel.LineNumber},
				}
				snippets = append(snippets, snippet)

				// Limit to avoid overwhelming the AI
				if len(snippets) >= 10 {
					break
				}
			}
		}
		if len(snippets) >= 10 {
			break
		}
	}

	return snippets
}

// buildBusinessImpact creates business impact assessment
func (ecb *EnhancedContextBuilder) buildBusinessImpact(businessCtx BusinessContext, entities []ChangedEntity) BusinessImpactInfo {
	return BusinessImpactInfo{
		Domain:          businessCtx.Domain,
		Criticality:     businessCtx.Criticality,
		UserImpact:      businessCtx.UserImpact,
		DataSensitivity: businessCtx.DataSensitivity,
		ComplianceAreas: businessCtx.ComplianceAreas,
		RiskAreas:       calculateRiskAreas(entities),
		Stakeholders:    identifyStakeholders(businessCtx.Domain),
	}
}

// buildArchitecturalContext creates architectural context
func (ecb *EnhancedContextBuilder) buildArchitecturalContext(archScope ArchitecturalScope) ArchitecturalContextInfo {
	return ArchitecturalContextInfo{
		Layer:            archScope.Layer,
		Components:       archScope.Components,
		Boundaries:       archScope.Boundaries,
		Patterns:         archScope.Patterns,
		DesignPrinciples: getRelevantDesignPrinciples(archScope.Layer),
		Constraints:      getArchitecturalConstraints(archScope.Layer),
	}
}

// buildQualityContext creates code quality context
func (ecb *EnhancedContextBuilder) buildQualityContext(style *ProjectStyleInfo, graph *DependencyGraph, entities []ChangedEntity) QualityContextInfo {
	return QualityContextInfo{
		ComplexityLevel:       calculateComplexityLevel(entities),
		TestabilityImpact:     calculateTestabilityImpact(entities),
		MaintainabilityImpact: calculateMaintainabilityImpact(entities),
		PerformanceImpact:     calculatePerformanceImpact(entities),
		QualityRisks:          identifyQualityRisks(entities),
		BestPractices:         getApplicableBestPractices(style),
		AntiPatterns:          getAntiPatternsToAvoid(style),
	}
}

// buildSecurityContext creates security context
func (ecb *EnhancedContextBuilder) buildSecurityContext(securityPatterns SecurityPatterns, entities []ChangedEntity) SecurityContextInfo {
	return SecurityContextInfo{
		SecurityLevel:    calculateOverallSecurityLevel(entities),
		ThreatAreas:      identifyThreatAreas(entities),
		SecurityPatterns: getRelevantSecurityPatterns(securityPatterns),
		ComplianceImpact: calculateComplianceImpact(entities),
		SecurityRisks:    identifySecurityRisks(entities),
		Mitigations:      suggestSecurityMitigations(entities),
	}
}

// buildReviewGuidance creates guidance for the AI reviewer
func (ecb *EnhancedContextBuilder) buildReviewGuidance(targetedCtx *TargetedContext) ReviewGuidanceInfo {
	return ReviewGuidanceInfo{
		PrimaryFocus:    determinePrimaryFocus(targetedCtx),
		SecondaryFocus:  determineSecondaryFocus(targetedCtx),
		CommonIssues:    identifyCommonIssues(targetedCtx),
		ProjectSpecific: getProjectSpecificChecks(targetedCtx.ProjectStyle),
		BusinessContext: generateBusinessContext(targetedCtx.BusinessImpact),
		ReviewStrategy:  determineReviewStrategy(targetedCtx),
		IgnorePatterns:  getIgnorePatterns(targetedCtx.ProjectStyle),
	}
}

// buildFocusAreas creates focus areas for review
func (ecb *EnhancedContextBuilder) buildFocusAreas(targetedCtx *TargetedContext) []FocusArea {
	var areas []FocusArea

	// Security focus area
	if targetedCtx.SecurityContext.SecurityLevel == "high" {
		areas = append(areas, FocusArea{
			Name:       "Security Review",
			Priority:   "high",
			Reason:     "Changes affect security-sensitive code",
			Specifics:  "Look for input validation, authentication, authorization issues",
			Examples:   "SQL injection, XSS, authentication bypass",
			Guidelines: "Verify all inputs are validated and outputs are sanitized",
		})
	}

	// Architecture focus area
	if len(targetedCtx.ArchitecturalContext.Boundaries) > 0 {
		areas = append(areas, FocusArea{
			Name:       "Architectural Consistency",
			Priority:   "medium",
			Reason:     "Changes cross architectural boundaries",
			Specifics:  "Verify layering and separation of concerns",
			Examples:   "Business logic in presentation layer, data access in business layer",
			Guidelines: "Maintain clear architectural boundaries",
		})
	}

	// Performance focus area
	if targetedCtx.QualityContext.PerformanceImpact == "high" {
		areas = append(areas, FocusArea{
			Name:       "Performance Impact",
			Priority:   "medium",
			Reason:     "Changes may impact performance",
			Specifics:  "Look for inefficient algorithms, N+1 queries, memory leaks",
			Examples:   "Loops in database queries, large object creation",
			Guidelines: "Consider algorithmic complexity and resource usage",
		})
	}

	return areas
}

// ConvertToPromptsContext converts TargetedContext to prompts.EnhancedContext
func (ecb *EnhancedContextBuilder) ConvertToPromptsContext(targetedCtx *TargetedContext) *prompts.EnhancedContext {
	// Convert our rich context to the format expected by the prompts package
	promptsCtx := &prompts.EnhancedContext{
		FilePath:  "", // Will be set by caller
		CleanDiff: "", // Will be set by caller
	}

	// Convert changed entities to enhanced function signatures with business context
	for _, entityCtx := range targetedCtx.ChangedEntities {
		if entityCtx.Entity.Type == EntityTypeFunction {
			// Create enhanced function signature with business and risk context
			sig := prompts.FunctionSignature{
				Name:       entityCtx.Entity.Name,
				IsExported: entityCtx.Entity.IsExported,
				LineNumber: entityCtx.Entity.StartLine,
			}

			// Add business context to parameters field for additional context
			var contextInfo []string
			if entityCtx.Entity.BusinessArea != "general" {
				contextInfo = append(contextInfo, fmt.Sprintf("Domain: %s", entityCtx.Entity.BusinessArea))
			}
			if entityCtx.RiskLevel != "low" {
				contextInfo = append(contextInfo, fmt.Sprintf("Risk: %s", entityCtx.RiskLevel))
			}
			if entityCtx.Entity.SecurityLevel == "high" {
				contextInfo = append(contextInfo, "Security: high")
			}
			if len(entityCtx.Dependents) > 0 {
				contextInfo = append(contextInfo, fmt.Sprintf("Dependents: %d", len(entityCtx.Dependents)))
			}

			if len(contextInfo) > 0 {
				sig.Parameters = contextInfo
			}

			promptsCtx.FunctionSignatures = append(promptsCtx.FunctionSignatures, sig)
		}

		// Convert types with enhanced context
		if entityCtx.Entity.Type == EntityTypeType || entityCtx.Entity.Type == EntityTypeStruct {
			typedef := prompts.TypeDefinition{
				Name:       entityCtx.Entity.Name,
				Type:       string(entityCtx.Entity.Type),
				IsExported: entityCtx.Entity.IsExported,
				LineNumber: entityCtx.Entity.StartLine,
			}

			// Add context info to fields
			var contextInfo []string
			if entityCtx.Entity.BusinessArea != "general" {
				contextInfo = append(contextInfo, fmt.Sprintf("Domain: %s", entityCtx.Entity.BusinessArea))
			}
			if entityCtx.RiskLevel != "low" {
				contextInfo = append(contextInfo, fmt.Sprintf("Risk: %s", entityCtx.RiskLevel))
			}
			typedef.Fields = contextInfo

			promptsCtx.TypeDefinitions = append(promptsCtx.TypeDefinitions, typedef)
		}
	}

	// Convert related code to related files with enhanced relationship context
	for _, relatedCode := range targetedCtx.RelatedCode {
		promptsCtx.RelatedFiles = append(promptsCtx.RelatedFiles, prompts.RelatedFile{
			Path:         relatedCode.FilePath,
			Relationship: fmt.Sprintf("%s (relevance: %s)", relatedCode.Relationship, relatedCode.Relevance),
			Snippet:      relatedCode.CodeSnippet,
		})
	}

	// Add business impact and architectural context as related files
	if targetedCtx.BusinessImpact.Domain != "" || targetedCtx.BusinessImpact.Criticality != "" {
		businessInfo := ecb.buildBusinessContextSnippet(targetedCtx.BusinessImpact)
		promptsCtx.RelatedFiles = append(promptsCtx.RelatedFiles, prompts.RelatedFile{
			Path:         "BUSINESS_CONTEXT",
			Relationship: "business_impact",
			Snippet:      businessInfo,
		})
	}

	// Add architectural context as a related file
	if targetedCtx.ArchitecturalContext.Layer != "" || len(targetedCtx.ArchitecturalContext.Components) > 0 {
		archInfo := ecb.buildArchitecturalContextSnippet(targetedCtx.ArchitecturalContext)
		promptsCtx.RelatedFiles = append(promptsCtx.RelatedFiles, prompts.RelatedFile{
			Path:         "ARCHITECTURAL_CONTEXT",
			Relationship: "architectural",
			Snippet:      archInfo,
		})
	}

	// Add quality and performance context
	if targetedCtx.QualityContext.PerformanceImpact == "high" || len(targetedCtx.QualityContext.QualityRisks) > 0 {
		qualityInfo := ecb.buildQualityContextSnippet(targetedCtx.QualityContext)
		promptsCtx.RelatedFiles = append(promptsCtx.RelatedFiles, prompts.RelatedFile{
			Path:         "QUALITY_CONTEXT",
			Relationship: "quality_analysis",
			Snippet:      qualityInfo,
		})
	}

	// Add focus areas as related files for high-priority guidance
	for _, focusArea := range targetedCtx.FocusAreas {
		if focusArea.Priority == "high" {
			focusInfo := ecb.buildFocusAreaSnippet(focusArea)
			promptsCtx.RelatedFiles = append(promptsCtx.RelatedFiles, prompts.RelatedFile{
				Path:         fmt.Sprintf("FOCUS_%s", strings.ToUpper(strings.ReplaceAll(focusArea.Name, " ", "_"))),
				Relationship: "priority_focus",
				Snippet:      focusInfo,
			})
		}
	}

	// Build ACTUAL usage patterns with real code examples from project style
	promptsCtx.UsagePatterns = ecb.buildMeaningfulUsagePatterns(targetedCtx.ProjectStyle, targetedCtx.ChangedEntities)

	// Add review guidance as usage patterns for strategic direction
	if targetedCtx.ReviewGuidance.PrimaryFocus != "" {
		promptsCtx.UsagePatterns = append(promptsCtx.UsagePatterns, prompts.UsagePattern{
			Pattern:      "review_strategy",
			Description:  fmt.Sprintf("Primary focus: %s", targetedCtx.ReviewGuidance.PrimaryFocus),
			Examples:     targetedCtx.ReviewGuidance.CommonIssues,
			BestPractice: targetedCtx.ReviewGuidance.ReviewStrategy,
		})
	}

	// Convert enhanced security context
	promptsCtx.SecurityContext = prompts.SecurityContext{
		HasAuthenticationLogic:  targetedCtx.SecurityContext.SecurityLevel == "high" || contains(targetedCtx.SecurityContext.ThreatAreas, "authentication"),
		HandlesUserInput:        contains(targetedCtx.SecurityContext.ThreatAreas, "input_validation"),
		AccessesDatabase:        contains(targetedCtx.SecurityContext.ThreatAreas, "database"),
		NetworkOperations:       contains(targetedCtx.SecurityContext.ThreatAreas, "network"),
		CryptographicOperations: contains(targetedCtx.SecurityContext.ThreatAreas, "cryptography"),
		HasInputValidation:      len(targetedCtx.SecurityContext.SecurityRisks) == 0, // Assume good validation if no risks
		HandlesFileOperations:   contains(targetedCtx.SecurityContext.ThreatAreas, "file_operations"),
	}

	// Convert semantic changes with enhanced business context
	for _, entityCtx := range targetedCtx.ChangedEntities {
		promptsCtx.SemanticChanges = append(promptsCtx.SemanticChanges, prompts.SemanticChange{
			Type:        mapEntityTypeToSemanticType(entityCtx.Entity.Type),
			Impact:      entityCtx.RiskLevel,
			Description: fmt.Sprintf("%s %s was %s (%s)", entityCtx.Entity.Type, entityCtx.Entity.Name, entityCtx.ChangeType, entityCtx.BusinessRelevance),
			Lines:       []int{entityCtx.Entity.StartLine, entityCtx.Entity.EndLine},
			Context:     fmt.Sprintf("Business area: %s, Security level: %s", entityCtx.Entity.BusinessArea, entityCtx.Entity.SecurityLevel),
		})
	}

	return promptsCtx
}

// buildMeaningfulUsagePatterns creates actually useful usage patterns with real code examples
func (ecb *EnhancedContextBuilder) buildMeaningfulUsagePatterns(projectStyle *ProjectStyleInfo, changedEntities []EntityContext) []prompts.UsagePattern {
	var patterns []prompts.UsagePattern

	// Only add patterns that have real value and examples
	if projectStyle == nil {
		return patterns
	}

	// Error handling patterns with actual examples
	if projectStyle.ErrorHandling.Pattern != "" {
		examples := ecb.buildErrorHandlingExamples(projectStyle.ErrorHandling)
		if len(examples) > 0 {
			patterns = append(patterns, prompts.UsagePattern{
				Pattern:      "error_handling",
				Description:  fmt.Sprintf("Project uses %s pattern for error handling", projectStyle.ErrorHandling.Pattern),
				Examples:     examples,
				BestPractice: ecb.buildErrorHandlingBestPractice(projectStyle.ErrorHandling),
			})
		}
	}

	// Function naming patterns with real examples
	if projectStyle.CodingConventions.NamingStyle.FunctionNaming != "" {
		examples := ecb.buildNamingExamples(changedEntities, projectStyle.CodingConventions.NamingStyle)
		if len(examples) > 0 {
			patterns = append(patterns, prompts.UsagePattern{
				Pattern:      "function_naming",
				Description:  fmt.Sprintf("Functions follow %s naming convention", projectStyle.CodingConventions.NamingStyle.FunctionNaming),
				Examples:     examples,
				BestPractice: fmt.Sprintf("Use %s for function names", projectStyle.CodingConventions.NamingStyle.FunctionNaming),
			})
		}
	}

	// Testing patterns with examples
	if projectStyle.TestingConventions.TestFramework != "" {
		examples := ecb.buildTestingExamples(projectStyle.TestingConventions)
		if len(examples) > 0 {
			patterns = append(patterns, prompts.UsagePattern{
				Pattern:      "testing_framework",
				Description:  fmt.Sprintf("Tests use %s framework with %s structure", projectStyle.TestingConventions.TestFramework, projectStyle.TestingConventions.TestStructure.TestTablePattern),
				Examples:     examples,
				BestPractice: fmt.Sprintf("Follow %s testing patterns established in the project", projectStyle.TestingConventions.TestFramework),
			})
		}
	}

	// Import organization patterns
	if projectStyle.CodingConventions.ImportStyle.GroupingStyle != "" {
		examples := ecb.buildImportExamples(projectStyle.CodingConventions.ImportStyle)
		if len(examples) > 0 {
			patterns = append(patterns, prompts.UsagePattern{
				Pattern:      "import_organization",
				Description:  fmt.Sprintf("Imports follow %s grouping style", projectStyle.CodingConventions.ImportStyle.GroupingStyle),
				Examples:     examples,
				BestPractice: "Group imports according to project conventions",
			})
		}
	}

	// Architecture patterns with examples
	if projectStyle.ArchitecturalStyle.LayerPattern != "" {
		examples := ecb.buildArchitectureExamples(projectStyle.ArchitecturalStyle)
		if len(examples) > 0 {
			patterns = append(patterns, prompts.UsagePattern{
				Pattern:      "architecture",
				Description:  fmt.Sprintf("Code follows %s architectural pattern", projectStyle.ArchitecturalStyle.LayerPattern),
				Examples:     examples,
				BestPractice: "Maintain architectural boundaries and layer separation",
			})
		}
	}

	return patterns
}

// buildErrorHandlingExamples creates real error handling examples based on project patterns
func (ecb *EnhancedContextBuilder) buildErrorHandlingExamples(errorHandling ErrorHandlingStyle) []string {
	var examples []string

	switch errorHandling.Pattern {
	case "explicit error returns":
		examples = []string{
			"func ProcessData(data []byte) (*Result, error) { if err := validate(data); err != nil { return nil, fmt.Errorf(\"validation failed: %w\", err) } }",
			"result, err := service.Process(input); if err != nil { return fmt.Errorf(\"processing failed: %w\", err) }",
		}
	case "try-catch or promises":
		examples = []string{
			"try { const result = await processData(data); } catch (error) { throw new Error(`Processing failed: ${error.message}`); }",
			"const result = await service.process(input).catch(err => { throw new ProcessingError(err); });",
		}
	case "exceptions":
		examples = []string{
			"try: result = process_data(data) except ValidationError as e: raise ProcessingError(f\"Failed to process: {e}\") from e",
			"def process_data(data): if not data: raise ValueError(\"Data cannot be empty\")",
		}
	case "Result<T, E> types":
		examples = []string{
			"fn process_data(data: &[u8]) -> Result<ProcessedData, ProcessingError> { validate_data(data)?; Ok(ProcessedData::new(data)) }",
			"let result = service.process(input).map_err(|e| ProcessingError::from(e))?;",
		}
	}

	return examples
}

// buildErrorHandlingBestPractice creates context-specific best practice guidance
func (ecb *EnhancedContextBuilder) buildErrorHandlingBestPractice(errorHandling ErrorHandlingStyle) string {
	switch errorHandling.Pattern {
	case "explicit error returns":
		return "Always check errors explicitly, wrap with context using fmt.Errorf, don't ignore errors"
	case "try-catch or promises":
		return "Use async/await with proper error handling, create specific error types, chain errors properly"
	case "exceptions":
		return "Use specific exception types, preserve original exception chain with 'from e', validate inputs early"
	case "Result<T, E> types":
		return "Use ? operator for error propagation, create specific error types, handle errors at appropriate levels"
	default:
		return "Follow project-specific error handling patterns consistently"
	}
}

// buildNamingExamples extracts real naming examples from changed entities
func (ecb *EnhancedContextBuilder) buildNamingExamples(changedEntities []EntityContext, naming NamingStyle) []string {
	var examples []string

	// Extract actual function names from changed entities as examples
	for _, entity := range changedEntities {
		if entity.Entity.Type == EntityTypeFunction && entity.Entity.Name != "" {
			if entity.Entity.IsExported {
				examples = append(examples, fmt.Sprintf("func %s() // exported function", entity.Entity.Name))
			} else {
				examples = append(examples, fmt.Sprintf("func %s() // internal function", entity.Entity.Name))
			}
		}
		if len(examples) >= 3 { // Limit to 3 examples
			break
		}
	}

	// Add some pattern-based examples if we don't have enough real ones
	if len(examples) < 2 {
		switch naming.FunctionNaming {
		case "camelCase":
			examples = append(examples, "func getUserData()", "func processPayment()", "func validateInput()")
		case "PascalCase":
			examples = append(examples, "func GetUserData()", "func ProcessPayment()", "func ValidateInput()")
		case "snake_case":
			examples = append(examples, "def get_user_data():", "def process_payment():", "def validate_input():")
		}
	}

	return examples
}

// buildTestingExamples creates testing pattern examples
func (ecb *EnhancedContextBuilder) buildTestingExamples(testing TestingConventions) []string {
	var examples []string

	switch testing.TestFramework {
	case "testing":
		examples = []string{
			"func TestUserService_CreateUser(t *testing.T) { /* table-driven test */ }",
			"func BenchmarkUserService_GetUser(b *testing.B) { /* benchmark test */ }",
		}
	case "testify":
		examples = []string{
			"func (suite *UserServiceSuite) TestCreateUser() { assert.NoError(suite.T(), err) }",
			"assert.Equal(t, expected, actual, \"values should match\")",
		}
	case "jest":
		examples = []string{
			"describe('UserService', () => { test('should create user', async () => { expect(result).toBeDefined(); }); });",
			"beforeEach(() => { mockUserRepository.mockClear(); });",
		}
	case "pytest":
		examples = []string{
			"def test_user_service_create_user(user_service): assert user_service.create_user(data) is not None",
			"@pytest.fixture def user_service(): return UserService(MockRepository())",
		}
	}

	return examples
}

// buildImportExamples creates import organization examples
func (ecb *EnhancedContextBuilder) buildImportExamples(importStyle ImportStyle) []string {
	var examples []string

	switch importStyle.GroupingStyle {
	case "stdlib/external/internal":
		examples = []string{
			"import (\n\t\"context\"\n\t\"fmt\"\n\n\t\"github.com/external/lib\"\n\n\t\"internal/pkg\"\n)",
		}
	case "grouped":
		examples = []string{
			"import { useState, useEffect } from 'react';\nimport { Button, Input } from '../components';",
		}
	}

	return examples
}

// buildArchitectureExamples creates architecture pattern examples
func (ecb *EnhancedContextBuilder) buildArchitectureExamples(arch ArchitecturalStyle) []string {
	var examples []string

	switch arch.LayerPattern {
	case "clean architecture":
		examples = []string{
			"// Domain layer: type User struct { ID string; Name string }",
			"// Use case: func (uc *UserUseCase) CreateUser(user User) error { return uc.repo.Save(user) }",
			"// Interface: type UserRepository interface { Save(User) error }",
		}
	case "layered":
		examples = []string{
			"// Controller -> Service -> Repository pattern",
			"func (c *UserController) CreateUser(w http.ResponseWriter, r *http.Request) { c.service.Create() }",
		}
	case "component-based":
		examples = []string{
			"const UserComponent = ({ user }: Props) => { return <div>{user.name}</div>; };",
			"export { UserComponent } from './UserComponent';",
		}
	}

	return examples
}

// mapEntityTypeToSemanticType maps entity types to semantic change types for prompts
func mapEntityTypeToSemanticType(entityType EntityType) string {
	switch entityType {
	case EntityTypeFunction, EntityTypeMethod:
		return "business_logic"
	case EntityTypeType, EntityTypeStruct, EntityTypeInterface:
		return "api_contract"
	case EntityTypeConst, EntityTypeVar:
		return "configuration"
	default:
		return "other"
	}
}

// Helper functions

func extractPackageFromPath(filePath string) string {
	parts := strings.Split(filePath, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return "unknown"
}

func inferBusinessAreaFromEntity(entity ChangedEntity) string {
	nameLower := strings.ToLower(entity.Name)
	if strings.Contains(nameLower, "auth") {
		return "authentication"
	}
	if strings.Contains(nameLower, "payment") {
		return "payment"
	}
	if strings.Contains(nameLower, "user") {
		return "user_management"
	}
	return "general"
}

func inferSecurityLevelFromEntity(entity ChangedEntity) string {
	combined := strings.ToLower(entity.Name + " " + entity.AfterCode)
	if strings.Contains(combined, "password") || strings.Contains(combined, "secret") {
		return "high"
	}
	if strings.Contains(combined, "auth") || strings.Contains(combined, "user") {
		return "medium"
	}
	return "low"
}

func calculateImpactLevel(rel Relationship) string {
	if rel.Strength > 0.8 {
		return "high"
	}
	if rel.Strength > 0.5 {
		return "medium"
	}
	return "low"
}

func calculateBreakingRisk(entity ChangedEntity, rel Relationship) string {
	if entity.ChangeType == ChangeTypeDeleted {
		return "high"
	}
	if entity.IsExported && rel.Strength > 0.7 {
		return "medium"
	}
	return "low"
}

func calculateBusinessRelevance(entity ChangedEntity) string {
	if strings.Contains(strings.ToLower(entity.Name), "business") ||
		strings.Contains(strings.ToLower(entity.Name), "service") {
		return "high"
	}
	if entity.IsExported {
		return "medium"
	}
	return "low"
}

func calculateRiskLevel(entity ChangedEntity, dependents []DependentContext) string {
	if entity.ChangeType == ChangeTypeDeleted && len(dependents) > 0 {
		return "high"
	}
	if entity.IsExported && len(dependents) > 3 {
		return "medium"
	}
	return "low"
}

func generateChangeExplanation(entity ChangedEntity) string {
	switch entity.ChangeType {
	case ChangeTypeAdded:
		return fmt.Sprintf("New %s '%s' was added", entity.Type, entity.Name)
	case ChangeTypeModified:
		return fmt.Sprintf("Existing %s '%s' was modified", entity.Type, entity.Name)
	case ChangeTypeDeleted:
		return fmt.Sprintf("Existing %s '%s' was deleted", entity.Type, entity.Name)
	default:
		return fmt.Sprintf("%s '%s' was changed", entity.Type, entity.Name)
	}
}

func calculateRiskAreas(entities []ChangedEntity) []string {
	var areas []string
	for _, entity := range entities {
		if entity.IsExported {
			areas = append(areas, "public_api")
		}
		if strings.Contains(strings.ToLower(entity.Name), "auth") {
			areas = append(areas, "authentication")
		}
	}
	return areas
}

func identifyStakeholders(domain string) []string {
	switch domain {
	case "authentication":
		return []string{"security_team", "backend_team"}
	case "payment":
		return []string{"finance_team", "security_team", "backend_team"}
	default:
		return []string{"backend_team"}
	}
}

func getRelevantDesignPrinciples(layer string) []string {
	switch layer {
	case "presentation":
		return []string{"separation_of_concerns", "single_responsibility"}
	case "business":
		return []string{"domain_driven_design", "clean_architecture"}
	case "data":
		return []string{"repository_pattern", "data_access_layer"}
	default:
		return []string{"solid_principles"}
	}
}

func getArchitecturalConstraints(layer string) []string {
	switch layer {
	case "presentation":
		return []string{"no_business_logic", "no_direct_data_access"}
	case "business":
		return []string{"no_ui_dependencies", "pure_business_logic"}
	case "data":
		return []string{"no_business_logic", "data_access_only"}
	default:
		return []string{}
	}
}

// More helper functions would be implemented here...
func calculateComplexityLevel(entities []ChangedEntity) string       { return "medium" }
func calculateTestabilityImpact(entities []ChangedEntity) string     { return "medium" }
func calculateMaintainabilityImpact(entities []ChangedEntity) string { return "medium" }
func calculatePerformanceImpact(entities []ChangedEntity) string     { return "low" }
func identifyQualityRisks(entities []ChangedEntity) []string         { return []string{} }
func getApplicableBestPractices(style *ProjectStyleInfo) []string    { return []string{} }
func getAntiPatternsToAvoid(style *ProjectStyleInfo) []string        { return []string{} }
func calculateOverallSecurityLevel(entities []ChangedEntity) string  { return "medium" }
func identifyThreatAreas(entities []ChangedEntity) []string          { return []string{} }
func getRelevantSecurityPatterns(patterns SecurityPatterns) []string { return []string{} }
func calculateComplianceImpact(entities []ChangedEntity) string      { return "low" }
func identifySecurityRisks(entities []ChangedEntity) []string        { return []string{} }
func suggestSecurityMitigations(entities []ChangedEntity) []string   { return []string{} }
func determinePrimaryFocus(ctx *TargetedContext) string              { return "functionality" }
func determineSecondaryFocus(ctx *TargetedContext) []string          { return []string{} }
func identifyCommonIssues(ctx *TargetedContext) []string             { return []string{} }
func getProjectSpecificChecks(style *ProjectStyleInfo) []string      { return []string{} }
func generateBusinessContext(impact BusinessImpactInfo) string       { return "" }
func determineReviewStrategy(ctx *TargetedContext) string            { return "comprehensive" }
func getIgnorePatterns(style *ProjectStyleInfo) []string             { return []string{} }

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// buildBusinessContextSnippet creates a business context snippet
func (ecb *EnhancedContextBuilder) buildBusinessContextSnippet(businessImpact BusinessImpactInfo) string {
	snippet := "BUSINESS IMPACT ANALYSIS:\n"
	if businessImpact.Domain != "" {
		snippet += fmt.Sprintf("Domain: %s\n", businessImpact.Domain)
	}
	if businessImpact.Criticality != "" {
		snippet += fmt.Sprintf("Criticality: %s\n", businessImpact.Criticality)
	}
	if businessImpact.UserImpact != "" {
		snippet += fmt.Sprintf("User Impact: %s\n", businessImpact.UserImpact)
	}
	if businessImpact.DataSensitivity != "" {
		snippet += fmt.Sprintf("Data Sensitivity: %s\n", businessImpact.DataSensitivity)
	}
	if len(businessImpact.RiskAreas) > 0 {
		snippet += fmt.Sprintf("Risk Areas: %s\n", strings.Join(businessImpact.RiskAreas, ", "))
	}
	if len(businessImpact.Stakeholders) > 0 {
		snippet += fmt.Sprintf("Stakeholders: %s\n", strings.Join(businessImpact.Stakeholders, ", "))
	}
	return snippet
}

// buildArchitecturalContextSnippet creates an architectural context snippet
func (ecb *EnhancedContextBuilder) buildArchitecturalContextSnippet(archCtx ArchitecturalContextInfo) string {
	snippet := "ARCHITECTURAL CONTEXT:\n"
	if archCtx.Layer != "" {
		snippet += fmt.Sprintf("Layer: %s\n", archCtx.Layer)
	}
	if len(archCtx.Components) > 0 {
		snippet += fmt.Sprintf("Components: %s\n", strings.Join(archCtx.Components, ", "))
	}
	if len(archCtx.Boundaries) > 0 {
		snippet += fmt.Sprintf("Boundaries: %s\n", strings.Join(archCtx.Boundaries, ", "))
	}
	if len(archCtx.Patterns) > 0 {
		snippet += fmt.Sprintf("Patterns: %s\n", strings.Join(archCtx.Patterns, ", "))
	}
	if len(archCtx.DesignPrinciples) > 0 {
		snippet += fmt.Sprintf("Design Principles: %s\n", strings.Join(archCtx.DesignPrinciples, ", "))
	}
	return snippet
}

// buildQualityContextSnippet creates a quality context snippet
func (ecb *EnhancedContextBuilder) buildQualityContextSnippet(qualityCtx QualityContextInfo) string {
	snippet := "QUALITY CONTEXT:\n"
	if qualityCtx.ComplexityLevel != "" {
		snippet += fmt.Sprintf("Complexity: %s\n", qualityCtx.ComplexityLevel)
	}
	if qualityCtx.PerformanceImpact != "" {
		snippet += fmt.Sprintf("Performance Impact: %s\n", qualityCtx.PerformanceImpact)
	}
	if qualityCtx.MaintainabilityImpact != "" {
		snippet += fmt.Sprintf("Maintainability Impact: %s\n", qualityCtx.MaintainabilityImpact)
	}
	if len(qualityCtx.QualityRisks) > 0 {
		snippet += fmt.Sprintf("Quality Risks: %s\n", strings.Join(qualityCtx.QualityRisks, ", "))
	}
	if len(qualityCtx.BestPractices) > 0 {
		snippet += fmt.Sprintf("Best Practices: %s\n", strings.Join(qualityCtx.BestPractices, ", "))
	}
	return snippet
}

// buildFocusAreaSnippet creates a focus area snippet
func (ecb *EnhancedContextBuilder) buildFocusAreaSnippet(focusArea FocusArea) string {
	snippet := fmt.Sprintf("FOCUS AREA: %s\n", focusArea.Name)
	snippet += fmt.Sprintf("Priority: %s\n", focusArea.Priority)
	snippet += fmt.Sprintf("Reason: %s\n", focusArea.Reason)
	if focusArea.Specifics != "" {
		snippet += fmt.Sprintf("Specifics: %s\n", focusArea.Specifics)
	}
	if focusArea.Examples != "" {
		snippet += fmt.Sprintf("Examples: %s\n", focusArea.Examples)
	}
	if focusArea.Guidelines != "" {
		snippet += fmt.Sprintf("Guidelines: %s\n", focusArea.Guidelines)
	}
	return snippet
}
