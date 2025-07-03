package prompts

import "github.com/maxbolgarin/codry/internal/model"

// LanguageConfig defines the target language for AI responses
type LanguageConfig struct {
	Language     model.Language `yaml:"language"`     // Language code (en, es, fr, de, ru, etc.)
	Instructions string         `yaml:"instructions"` // Language-specific instructions for the AI
}

// DefaultLanguages provides common language configurations
var DefaultLanguages = map[model.Language]LanguageConfig{
	model.LanguageEnglish: {
		Language:     model.LanguageEnglish,
		Instructions: "Respond in clear, professional English. Use technical terminology appropriately.",
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
