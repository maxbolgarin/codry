package astparser

import (
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/bash"
	"github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/csharp"
	"github.com/smacker/go-tree-sitter/css"
	"github.com/smacker/go-tree-sitter/cue"
	"github.com/smacker/go-tree-sitter/dockerfile"
	"github.com/smacker/go-tree-sitter/elixir"
	"github.com/smacker/go-tree-sitter/elm"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/groovy"
	"github.com/smacker/go-tree-sitter/hcl"
	"github.com/smacker/go-tree-sitter/html"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/kotlin"
	"github.com/smacker/go-tree-sitter/lua"
	tree_sitter_markdown "github.com/smacker/go-tree-sitter/markdown/tree-sitter-markdown"
	"github.com/smacker/go-tree-sitter/ocaml"
	"github.com/smacker/go-tree-sitter/php"
	"github.com/smacker/go-tree-sitter/protobuf"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/scala"
	"github.com/smacker/go-tree-sitter/sql"
	"github.com/smacker/go-tree-sitter/svelte"
	"github.com/smacker/go-tree-sitter/swift"
	"github.com/smacker/go-tree-sitter/toml"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
	// "github.com/smacker/go-tree-sitter/yaml" // Commented out due to compilation issues with cassert header
)

type ProgrammingLanguage string

const (
	// Popular General-purpose Languages
	LanguageJavaScript ProgrammingLanguage = "javascript"
	LanguageTypeScript ProgrammingLanguage = "typescript"
	LanguageTSX        ProgrammingLanguage = "tsx"
	LanguagePython     ProgrammingLanguage = "python"
	LanguageJava       ProgrammingLanguage = "java"
	LanguageGo         ProgrammingLanguage = "go"
	LanguageC          ProgrammingLanguage = "c"
	LanguageCpp        ProgrammingLanguage = "cpp"
	LanguageCSharp     ProgrammingLanguage = "csharp"
	LanguagePHP        ProgrammingLanguage = "php"
	LanguageRuby       ProgrammingLanguage = "ruby"
	LanguageSwift      ProgrammingLanguage = "swift"
	LanguageKotlin     ProgrammingLanguage = "kotlin"
	LanguageRust       ProgrammingLanguage = "rust"
	LanguageScala      ProgrammingLanguage = "scala"
	LanguageElixir     ProgrammingLanguage = "elixir"
	LanguageGroovy     ProgrammingLanguage = "groovy"
	LanguageLua        ProgrammingLanguage = "lua"
	LanguageElm        ProgrammingLanguage = "elm"
	LanguageOCaml      ProgrammingLanguage = "ocaml"
	LanguageSvelte     ProgrammingLanguage = "svelte"

	// Web & Markup Languages
	LanguageHTML     ProgrammingLanguage = "html"
	LanguageCSS      ProgrammingLanguage = "css"
	LanguageMarkdown ProgrammingLanguage = "markdown"

	// Data, Config, and Query Languages
	LanguageSQL        ProgrammingLanguage = "sql"
	LanguageYAML       ProgrammingLanguage = "yaml"
	LanguageTOML       ProgrammingLanguage = "toml"
	LanguageCue        ProgrammingLanguage = "cue"
	LanguageHCL        ProgrammingLanguage = "hcl"
	LanguageProtobuf   ProgrammingLanguage = "protobuf"
	LanguageDockerfile ProgrammingLanguage = "dockerfile"
	LanguageBash       ProgrammingLanguage = "bash"

	// Without parsers
	LanguageText ProgrammingLanguage = "text"
	LanguageJSON ProgrammingLanguage = "json"
	LanguageXML  ProgrammingLanguage = "xml"
)

var languagesParsers = map[ProgrammingLanguage]*sitter.Language{
	// Popular General-purpose Languages
	LanguageJavaScript: javascript.GetLanguage(),
	LanguageTypeScript: typescript.GetLanguage(),
	LanguageTSX:        tsx.GetLanguage(),
	LanguagePython:     python.GetLanguage(),
	LanguageJava:       java.GetLanguage(),
	LanguageGo:         golang.GetLanguage(),
	LanguageC:          c.GetLanguage(),
	LanguageCpp:        cpp.GetLanguage(),
	LanguageCSharp:     csharp.GetLanguage(),
	LanguagePHP:        php.GetLanguage(),
	LanguageRuby:       ruby.GetLanguage(),
	LanguageSwift:      swift.GetLanguage(),
	LanguageKotlin:     kotlin.GetLanguage(),
	LanguageRust:       rust.GetLanguage(),
	LanguageScala:      scala.GetLanguage(),
	LanguageElixir:     elixir.GetLanguage(),
	LanguageGroovy:     groovy.GetLanguage(),
	LanguageLua:        lua.GetLanguage(),
	LanguageElm:        elm.GetLanguage(),
	LanguageOCaml:      ocaml.GetLanguage(),
	LanguageSvelte:     svelte.GetLanguage(),

	// Web & Markup Languages
	LanguageHTML:     html.GetLanguage(),
	LanguageCSS:      css.GetLanguage(),
	LanguageMarkdown: tree_sitter_markdown.GetLanguage(),

	// Data, Config, and Query Languages
	LanguageSQL: sql.GetLanguage(),
	// LanguageYAML:       yaml.GetLanguage(), // Commented out due to compilation issues
	LanguageTOML:       toml.GetLanguage(),
	LanguageCue:        cue.GetLanguage(),
	LanguageHCL:        hcl.GetLanguage(),
	LanguageProtobuf:   protobuf.GetLanguage(),
	LanguageDockerfile: dockerfile.GetLanguage(),
	LanguageBash:       bash.GetLanguage(),
}

// Map file extensions to language identifiers for markdown syntax highlighting
var languageExtensions = map[string]ProgrammingLanguage{
	// Go
	".go": LanguageGo,

	// JavaScript/TypeScript
	".js":  LanguageJavaScript,
	".jsx": LanguageTSX,
	".mjs": LanguageJavaScript,
	".cjs": LanguageJavaScript,
	".ts":  LanguageTypeScript,
	".tsx": LanguageTSX,

	// Python
	".py":  LanguagePython,
	".pyw": LanguagePython,
	".pyi": LanguagePython,
	".pyx": LanguagePython,

	// Java
	".java": LanguageJava,

	// Kotlin
	".kt":  LanguageKotlin,
	".kts": LanguageKotlin,

	// C
	".c": LanguageC,
	".h": LanguageC,

	// C++
	".cpp": LanguageCpp,
	".cxx": LanguageCpp,
	".cc":  LanguageCpp,
	".hpp": LanguageCpp,
	".hxx": LanguageCpp,
	".hh":  LanguageCpp,

	// C#
	".cs":  LanguageCSharp,
	".csx": LanguageCSharp,

	// PHP
	".php":   LanguagePHP,
	".phtml": LanguagePHP,
	".php3":  LanguagePHP,
	".php4":  LanguagePHP,
	".php5":  LanguagePHP,
	".phps":  LanguagePHP,
	".phpt":  LanguagePHP,

	// Ruby
	".rb":      LanguageRuby,
	".rbw":     LanguageRuby,
	".gemspec": LanguageRuby,
	".rake":    LanguageRuby,

	// Swift
	".swift": LanguageSwift,

	// Rust
	".rs": LanguageRust,

	// Scala
	".scala": LanguageScala,
	".sc":    LanguageScala,

	// Elixir
	".ex":  LanguageElixir,
	".exs": LanguageElixir,

	// Groovy
	".groovy": LanguageGroovy,
	".gvy":    LanguageGroovy,
	".gy":     LanguageGroovy,
	".gsh":    LanguageGroovy,

	// Lua
	".lua": LanguageLua,

	// Elm
	".elm": LanguageElm,

	// OCaml
	".ml":  LanguageOCaml,
	".mli": LanguageOCaml,
	".ml4": LanguageOCaml,
	".mll": LanguageOCaml,
	".mly": LanguageOCaml,

	// Svelte
	".svelte": LanguageSvelte,

	// Web & Markup
	".html":     LanguageHTML,
	".htm":      LanguageHTML,
	".xhtml":    LanguageHTML,
	".css":      LanguageCSS,
	".scss":     LanguageCSS,
	".sass":     LanguageCSS,
	".less":     LanguageCSS,
	".md":       LanguageMarkdown,
	".markdown": LanguageMarkdown,
	".mdown":    LanguageMarkdown,
	".mkd":      LanguageMarkdown,
	".mkdn":     LanguageMarkdown,
	".mdwn":     LanguageMarkdown,
	".mdtxt":    LanguageMarkdown,
	".mdtext":   LanguageMarkdown,
	".text":     LanguageMarkdown,
	".rmd":      LanguageMarkdown,

	// Data, Config, and Query
	".sql":        LanguageSQL,
	".yaml":       LanguageYAML,
	".yml":        LanguageYAML,
	".toml":       LanguageTOML,
	".cue":        LanguageCue,
	".hcl":        LanguageHCL,
	".proto":      LanguageProtobuf,
	".dockerfile": LanguageDockerfile,
	"dockerfile":  LanguageDockerfile,
	".bash":       LanguageBash,
	".sh":         LanguageBash,
	".zsh":        LanguageBash,
	".fish":       LanguageBash,

	// Misc
	".txt":  LanguageText,
	".json": LanguageJSON,
	".xml":  LanguageXML,

	// Other (fallbacks for supported languages)
	".ini":  LanguageText,
	".cfg":  LanguageText,
	".conf": LanguageText,
	".bat":  LanguageBash, // not perfect, but closest parser
}

// DetectProgrammingLanguage detects programming language from file path
func DetectProgrammingLanguage(filePath string) ProgrammingLanguage {
	if filePath == "" {
		return LanguageText
	}

	// Get the file extension (including the dot)
	ext := strings.ToLower(filepath.Ext(filePath))

	// Special case for common filenames without extensions
	fileName := strings.ToLower(filepath.Base(filePath))
	switch fileName {
	case "dockerfile":
		return LanguageDockerfile
	case "makefile":
		return LanguageBash
	case "gemfile":
		return LanguageRuby
	case "rakefile":
		return LanguageRuby
	case "package.json":
		return LanguageJSON
	case "composer.json":
		return LanguageJSON
	case ".gitignore", ".dockerignore", ".eslintignore":
		return LanguageText
	case ".env", ".env.example":
		return LanguageBash
	}

	// Look up the extension in our language map
	if language, exists := languageExtensions[ext]; exists {
		return language
	}

	// If we can't determine the language, return a generic text format
	return LanguageText
}

var symbolNodes = map[string]bool{
	// Go
	"function_declaration": true,
	"method_declaration":   true,
	"type_declaration":     true,
	"type_spec":            true,
	"var_declaration":      true,
	"var_spec":             true,
	"const_declaration":    true,
	"const_spec":           true,
	"interface_type":       true,
	"struct_type":          true,
	"func_literal":         true,
	"method_spec":          true,
	"function_spec":        true,

	// JavaScript/TypeScript
	"function_expression":    true,
	"arrow_function":         true,
	"method_definition":      true,
	"variable_declaration":   true,
	"variable_declarator":    true,
	"type_alias_declaration": true,
	"namespace_declaration":  true,
	"module_declaration":     true,
	"export_statement":       true,
	"import_statement":       true,
	"lexical_declaration":    true,

	// Python
	"function_definition": true,
	"async_function_def":  true,
	"decorated_definition": true,

	// Java
	"constructor_declaration":    true,
	"local_variable_declaration": true,
	"field_declaration":          true,

	// C/C++
	"function_definition":                   true,
	"constructor_or_destructor_declaration": true,
	"struct_specifier":                      true,
	"union_specifier":                       true,
	"enum_specifier":                        true,
	"function_declarator":                   true,
	"declaration":                           true,

	// C#
	"property_declaration":   true,
	"struct_declaration":     true,

	// PHP
	"function_definition":    true,

	// Ruby
	"method":                 true,
	"module":                 true,
	"def":                    true,
	"class_definition":       true,
	"module_definition":      true,
	"method_definition":      true,

	// Rust
	"function_item":          true,
	"struct_item":            true,
	"enum_item":              true,
	"trait_item":             true,
	"impl_item":              true,
	"mod_item":               true,
	"const_item":             true,
	"static_item":            true,

	// Swift
	"struct_declaration":     true,
	"enum_declaration":       true,
	"protocol_declaration":   true,
	"variable_declaration":   true,
	"constant_declaration":   true,

	// Kotlin
	"object_declaration":     true,

	// Scala
	"object_definition":      true,
	"trait_definition":       true,
	"val_definition":         true,
	"var_definition":         true,

	// Elixir
	"function":               true,
	"defp":                   true,
	"defmodule":              true,
	"defstruct":              true,
	"defprotocol":            true,

	// Groovy
	"variable_declaration":   true,

	// Lua
	"function_statement":     true,
	"local_function_statement": true,
	"local_statement":        true,

	// Elm
	"function_declaration":   true,
	"type_declaration":       true,
	"type_alias_declaration": true,

	// OCaml
	"value_definition":       true,
	"type_definition":        true,
	"module_definition":      true,
	"exception_definition":   true,

	// Svelte
	"script":                 true,
	"component":              true,

	// Generic patterns that might be missed
	"definition":             true,
	"specification":          true,
	"declarator":             true,
}
