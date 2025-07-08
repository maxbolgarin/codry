package prompts

import (
	"strings"

	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/logze/v2"
)

// LanguageConfig defines the target language for AI responses
type LanguageConfig struct {
	Language     model.Language `yaml:"language"`     // Language code (en, es, fr, de, ru, etc.)
	Instructions string         `yaml:"instructions"` // Language-specific instructions for the AI

	DescriptionHeaders        DescriptionHeaders        `yaml:"description_headers"`
	ListOfChangesHeaders      ListOfChangesHeaders      `yaml:"list_of_changes_headers"`
	ArchitectureReviewHeaders ArchitectureReviewHeaders `yaml:"architecture_review_headers"`
	CodeReviewHeaders         CodeReviewHeaders         `yaml:"code_review_headers"`
}

type DescriptionHeaders struct {
	Title                    string `yaml:"title"`
	NewFeaturesHeader        string `yaml:"new_features_header"`
	BugFixesHeader           string `yaml:"bug_fixes_header"`
	RefactoringHeader        string `yaml:"refactoring_header"`
	TestingHeader            string `yaml:"testing_header"`
	CIAndBuildHeader         string `yaml:"ci_and_build_header"`
	DocsImprovementHeader    string `yaml:"docs_improvement_header"`
	RemovalsAndCleanupHeader string `yaml:"removals_and_cleanup_header"`
	OtherChangesHeader       string `yaml:"other_changes_header"`
}

type ListOfChangesHeaders struct {
	Title       string `yaml:"general_header"`
	TableHeader string `yaml:"table_header"`

	FeatureTypeText            string `yaml:"feature_type_text"`
	BugFixTypeText             string `yaml:"bug_fix_type_text"`
	RefactorTypeText           string `yaml:"refactor_type_text"`
	TestTypeText               string `yaml:"test_type_text"`
	DeployTypeText             string `yaml:"deploy_type_text"`
	ConfigTypeText             string `yaml:"config_type_text"`
	DocsImprovementTypeText    string `yaml:"docs_improvement_type_text"`
	RemovalsAndCleanupTypeText string `yaml:"removals_and_cleanup_type_text"`
	StyleTypeText              string `yaml:"style_type_text"`
	OtherChangesTypeText       string `yaml:"other_changes_type_text"`
}

type ArchitectureReviewHeaders struct {
	GeneralHeader            string `yaml:"general_header"`
	ArchitectureIssuesHeader string `yaml:"architecture_issues_header"`
	PerformanceIssuesHeader  string `yaml:"performance_issues_header"`
	SecurityIssuesHeader     string `yaml:"security_issues_header"`
	DocsImprovementHeader    string `yaml:"docs_improvement_header"`
}

type CodeReviewHeaders struct {
	CriticalIssueHeader          string `yaml:"critical_issue_header"`
	PotentialBugHeader           string `yaml:"potential_issue_header"`
	PerformanceImprovementHeader string `yaml:"performance_improvement_header"`
	SecurityImprovementHeader    string `yaml:"security_improvement_header"`
	RefactorSuggestionHeader     string `yaml:"refactor_suggestion_header"`
	OtherIssueHeader             string `yaml:"other_issue_header"`

	ConfidenceHeader string `yaml:"confidence_header"`
	PriorityHeader   string `yaml:"priority_header"`
	SuggestionHeader string `yaml:"suggestion_header"`

	ConfidenceLow      string `yaml:"confidence_low"`
	ConfidenceMedium   string `yaml:"confidence_medium"`
	ConfidenceHigh     string `yaml:"confidence_high"`
	ConfidenceVeryHigh string `yaml:"confidence_very_high"`

	PriorityLow      string `yaml:"priority_low"`
	PriorityMedium   string `yaml:"priority_medium"`
	PriorityHigh     string `yaml:"priority_high"`
	PriorityCritical string `yaml:"priority_critical"`
}

// DefaultLanguages provides common language configurations
var DefaultLanguages = map[model.Language]LanguageConfig{
	model.LanguageEnglish: {
		Language:     model.LanguageEnglish,
		Instructions: "Respond in clear, professional English. Use technical terminology appropriately.",

		DescriptionHeaders: DescriptionHeaders{
			Title:                    "🤖 Review Summary",
			NewFeaturesHeader:        "⚡️ New features",
			BugFixesHeader:           "🐛 Bug fixes",
			RefactoringHeader:        "🛠️ Refactoring",
			TestingHeader:            "🧪 Testing",
			CIAndBuildHeader:         "🔧 CI/CD",
			DocsImprovementHeader:    "📚 Documentation",
			RemovalsAndCleanupHeader: "🧹 Removals and cleanup",
			OtherChangesHeader:       "🔄 Other changes",
		},
		ListOfChangesHeaders: ListOfChangesHeaders{
			Title:       "📝 List of changes",
			TableHeader: "| File | Change type | Diff | Description |",

			FeatureTypeText:            "⚡️ New feature",
			BugFixTypeText:             "🐛 Bug fix",
			RefactorTypeText:           "🛠️ Refactoring",
			TestTypeText:               "🧪 Testing",
			DeployTypeText:             "🔧 Deployment",
			DocsImprovementTypeText:    "📚 Documentation",
			RemovalsAndCleanupTypeText: "🧹 Removals",
			StyleTypeText:              "🎨 Style",
			OtherChangesTypeText:       "🔄 Other changes",
		},
		ArchitectureReviewHeaders: ArchitectureReviewHeaders{
			GeneralHeader:            "🏗️ Architecture review",
			ArchitectureIssuesHeader: "🚨 Architecture issues",
			PerformanceIssuesHeader:  "🚀 Performance issues",
			SecurityIssuesHeader:     "🔒 Security issues",
			DocsImprovementHeader:    "📚 Documentation",
		},

		CodeReviewHeaders: CodeReviewHeaders{
			CriticalIssueHeader:          "🚨 Critical issue",
			PotentialBugHeader:           "⚠️ Potential bug",
			PerformanceImprovementHeader: "🚀 Performance improvement",
			SecurityImprovementHeader:    "🔒 Security improvement",
			RefactorSuggestionHeader:     "🛠️ Refactor suggestion",
			OtherIssueHeader:             "🔄 Other issue",

			SuggestionHeader: "💡 Suggestion",
			ConfidenceHeader: "Model confidence",
			PriorityHeader:   "Issue priority",

			PriorityLow:      "backlog ⚪️",
			PriorityMedium:   "could be fixed later 🟢",
			PriorityHigh:     "should be fixed soon 🟡",
			PriorityCritical: "must be fixed immediately 🔴",

			ConfidenceLow:      "low (20-40%)",
			ConfidenceMedium:   "medium (40-70%)",
			ConfidenceHigh:     "high (70-90%)",
			ConfidenceVeryHigh: "very high (90-100%)",
		},
	},
	model.LanguageSpanish: {
		Language:     model.LanguageSpanish,
		Instructions: "Responde en español claro y profesional. Usa terminología técnica apropiada.",
	},
	model.LanguageFrench: {
		Language:     model.LanguageFrench,
		Instructions: "Répondez en français clair et professionnel. Utilisez une terminologie technique appropriée.",
	},
	model.LanguageGerman: {
		Language:     model.LanguageGerman,
		Instructions: "Antworten Sie in klarem, professionellem Deutsch. Verwenden Sie angemessene technische Terminologie.",
	},
	model.LanguageRussian: {
		Language:     model.LanguageRussian,
		Instructions: "Отвечайте на русском языке четко и профессионально. Используйте соответствующую техническую терминологию.",
	},
	model.LanguagePortuguese: {
		Language:     model.LanguagePortuguese,
		Instructions: "Responda em português claro e profissional. Use terminologia técnica apropriada.",
	},
	model.LanguageItalian: {
		Language:     model.LanguageItalian,
		Instructions: "Rispondi in italiano chiaro e professionale. Usa una terminologia tecnica appropriata.",
	},
	model.LanguageJapanese: {
		Language:     model.LanguageJapanese,
		Instructions: "明確で専門的な日本語で回答してください。適切な技術用語を使用してください。",
	},
	model.LanguageKorean: {
		Language:     model.LanguageKorean,
		Instructions: "명확하고 전문적인 한국어로 답변해 주세요. 적절한 기술 용어를 사용해 주세요.",
	},
	model.LanguageChinese: {
		Language:     model.LanguageChinese,
		Instructions: "请用清晰、专业的中文回答。适当使用技术术语。",
	},
}

func (lch ListOfChangesHeaders) GetByType(t model.FileChangeType) string {
	switch t {
	case model.FileChangeTypeNewFeature:
		return lch.FeatureTypeText
	case model.FileChangeTypeBugFix:
		return lch.BugFixTypeText
	case model.FileChangeTypeRefactor:
		return lch.RefactorTypeText
	case model.FileChangeTypeTest:
		return lch.TestTypeText
	case model.FileChangeTypeDeploy:
		return lch.DeployTypeText
	case model.FileChangeTypeDocs:
		return lch.DocsImprovementTypeText
	case model.FileChangeTypeCleanup:
		return lch.RemovalsAndCleanupTypeText
	case model.FileChangeTypeStyle:
		return lch.StyleTypeText
	case model.FileChangeTypeOther:
		return lch.OtherChangesTypeText
	}
	logze.Warn("unknown file change type", "file_change_type", t)
	return lch.OtherChangesTypeText
}

func (dh CodeReviewHeaders) GetByType(t model.IssueType) string {
	if contains(string(t), "reliability", "style") {
		return dh.RefactorSuggestionHeader
	}

	switch t {
	case model.IssueTypeCritical:
		return dh.CriticalIssueHeader
	case model.IssueTypeBug:
		return dh.PotentialBugHeader
	case model.IssueTypePerformance:
		return dh.PerformanceImprovementHeader
	case model.IssueTypeSecurity:
		return dh.SecurityImprovementHeader
	case model.IssueTypeRefactor:
		return dh.RefactorSuggestionHeader
	case model.IssueTypeOther:
		return dh.OtherIssueHeader
	}

	logze.Warn("unknown issue type", "issue_type", t)
	return dh.OtherIssueHeader
}

func (dh CodeReviewHeaders) GetConfidence(c model.ReviewConfidence) string {
	switch c {
	case model.ConfidenceVeryHigh:
		return dh.ConfidenceVeryHigh
	case model.ConfidenceHigh:
		return dh.ConfidenceHigh
	case model.ConfidenceMedium:
		return dh.ConfidenceMedium
	case model.ConfidenceLow:
		return dh.ConfidenceLow
	}
	logze.Warn("unknown confidence", "confidence", c)
	return dh.ConfidenceMedium
}

func (dh CodeReviewHeaders) GetPriority(s model.ReviewPriority) string {
	switch s {
	case model.ReviewPriorityCritical:
		return dh.PriorityCritical
	case model.ReviewPriorityHigh:
		return dh.PriorityHigh
	case model.ReviewPriorityMedium:
		return dh.PriorityMedium
	case model.ReviewPriorityBacklog:
		return dh.PriorityLow
	}
	logze.Warn("unknown priority", "priority", s)
	return dh.PriorityMedium
}

func contains(item string, slice ...string) bool {
	for _, s := range slice {
		if strings.Contains(strings.ToLower(item), strings.ToLower(s)) {
			return true
		}
	}
	return false
}
