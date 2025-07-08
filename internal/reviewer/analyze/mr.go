package analyze

// // convertToPromptsContext converts reviewer.EnhancedContext to prompts.EnhancedContext
// func convertToPromptsContext(ctx *EnhancedContext) *prompts.EnhancedContext {
// 	return &prompts.EnhancedContext{
// 		FilePath:         ctx.FilePath,
// 		FileContent:      ctx.FileContent,
// 		CleanDiff:        ctx.CleanDiff,
// 		ImportedPackages: ctx.ImportedPackages,
// 		RelatedFiles: func() []prompts.RelatedFile {
// 			var converted []prompts.RelatedFile
// 			for _, rf := range ctx.RelatedFiles {
// 				converted = append(converted, prompts.RelatedFile{
// 					Path:         rf.Path,
// 					Relationship: rf.Relationship,
// 					Snippet:      rf.Snippet,
// 				})
// 			}
// 			return converted
// 		}(),
// 		FunctionSignatures: func() []prompts.FunctionSignature {
// 			var converted []prompts.FunctionSignature
// 			for _, fs := range ctx.FunctionSignatures {
// 				converted = append(converted, prompts.FunctionSignature{
// 					Name:       fs.Name,
// 					Parameters: fs.Parameters,
// 					Returns:    fs.Returns,
// 					IsExported: fs.IsExported,
// 					LineNumber: fs.LineNumber,
// 				})
// 			}
// 			return converted
// 		}(),
// 		TypeDefinitions: func() []prompts.TypeDefinition {
// 			var converted []prompts.TypeDefinition
// 			for _, td := range ctx.TypeDefinitions {
// 				converted = append(converted, prompts.TypeDefinition{
// 					Name:       td.Name,
// 					Type:       td.Type,
// 					Fields:     td.Fields,
// 					Methods:    td.Methods,
// 					IsExported: td.IsExported,
// 					LineNumber: td.LineNumber,
// 				})
// 			}
// 			return converted
// 		}(),
// 		UsagePatterns: func() []prompts.UsagePattern {
// 			var converted []prompts.UsagePattern
// 			for _, up := range ctx.UsagePatterns {
// 				converted = append(converted, prompts.UsagePattern{
// 					Pattern:      up.Pattern,
// 					Description:  up.Description,
// 					Examples:     up.Examples,
// 					BestPractice: up.BestPractice,
// 				})
// 			}
// 			return converted
// 		}(),
// 		SecurityContext: prompts.SecurityContext{
// 			HasAuthenticationLogic:  ctx.SecurityContext.HasAuthenticationLogic,
// 			HasInputValidation:      ctx.SecurityContext.HasInputValidation,
// 			HandlesUserInput:        ctx.SecurityContext.HandlesUserInput,
// 			AccessesDatabase:        ctx.SecurityContext.AccessesDatabase,
// 			HandlesFileOperations:   ctx.SecurityContext.HandlesFileOperations,
// 			NetworkOperations:       ctx.SecurityContext.NetworkOperations,
// 			CryptographicOperations: ctx.SecurityContext.CryptographicOperations,
// 		},
// 		SemanticChanges: func() []prompts.SemanticChange {
// 			var converted []prompts.SemanticChange
// 			for _, sc := range ctx.SemanticChanges {
// 				converted = append(converted, prompts.SemanticChange{
// 					Type:        sc.Type,
// 					Impact:      sc.Impact,
// 					Description: sc.Description,
// 					Lines:       sc.Lines,
// 					Context:     sc.Context,
// 				})
// 			}
// 			return converted
// 		}(),
// 	}
// }

// // cleanDiffSnippet removes diff prefixes (+, -, space) from code snippet and normalizes indentation
// func cleanDiffSnippet(snippet string) string {
// 	lines := strings.Split(snippet, "\n")
// 	var cleaned []string

// 	if len(lines) == 0 {
// 		return ""
// 	}

// 	// First pass: remove diff prefixes and collect non-empty lines
// 	var processedLines []string
// 	for _, line := range lines {
// 		if len(line) == 0 {
// 			processedLines = append(processedLines, "")
// 			continue
// 		}

// 		// Remove diff prefixes: +, -, or space at the beginning
// 		cleanLine := line
// 		if line[0] == '+' || line[0] == '-' || line[0] == ' ' {
// 			cleanLine = line[1:]
// 		}
// 		processedLines = append(processedLines, cleanLine)
// 	}

// 	// // Second pass: find minimum leading whitespace (excluding empty lines)
// 	// minIndent := -1
// 	// for _, line := range processedLines {
// 	// 	if len(line) == 0 {
// 	// 		continue // Skip empty lines when calculating minimum indent
// 	// 	}

// 	// 	indent := 0
// 	// 	for i := 0; i < len(line); i++ {
// 	// 		if line[i] == ' ' || line[i] == '\t' {
// 	// 			if line[i] == '\t' {
// 	// 				indent += 4 // Count tabs as 4 spaces for normalization
// 	// 			} else {
// 	// 				indent++
// 	// 			}
// 	// 		} else {
// 	// 			break
// 	// 		}
// 	// 	}

// 	// 	if minIndent == -1 || indent < minIndent {
// 	// 		minIndent = indent
// 	// 	}
// 	// }

// 	// // If no indentation found, set to 0
// 	// if minIndent == -1 {
// 	// 	minIndent = 0
// 	// }

// 	// // Third pass: remove the minimum indentation from all lines
// 	// for _, line := range processedLines {
// 	// 	if len(line) == 0 {
// 	// 		cleaned = append(cleaned, "")
// 	// 		continue
// 	// 	}

// 	// 	// Remove leading whitespace up to minIndent
// 	// 	currentIndent := 0
// 	// 	startIndex := 0
// 	// 	for i := 0; i < len(line) && currentIndent < minIndent; i++ {
// 	// 		if line[i] == ' ' {
// 	// 			currentIndent++
// 	// 			startIndex = i + 1
// 	// 		} else if line[i] == '\t' {
// 	// 			currentIndent += 4 // Count tabs as 4 spaces
// 	// 			startIndex = i + 1
// 	// 		} else {
// 	// 			break
// 	// 		}
// 	// 	}

// 	// 	if startIndex < len(line) {
// 	// 		cleaned = append(cleaned, line[startIndex:])
// 	// 	} else {
// 	// 		cleaned = append(cleaned, "")
// 	// 	}
// 	// }

// 	return strings.Join(cleaned, "\n")
// }
