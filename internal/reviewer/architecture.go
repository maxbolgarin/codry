package reviewer

import (
	"context"
	"strings"

	"github.com/maxbolgarin/codry/internal/agent/prompts"
	"github.com/maxbolgarin/codry/internal/model"
	"github.com/maxbolgarin/errm"
	"github.com/maxbolgarin/logze/v2"
)

func (s *Reviewer) createArchitectureReview(ctx context.Context, request model.ReviewRequest, fullDiff string, log logze.Logger) error {
	if !s.cfg.EnableArchitectureReview {
		log.Info("architecture review is disabled, skipping")
		return nil
	}

	log.Info("generating architecture review")

	architectureResult, err := s.agent.GenerateArchitectureReview(ctx, fullDiff)
	if err != nil {
		return errm.Wrap(err, "failed to generate architecture review")
	}

	comment := &model.Comment{
		Body: architectureResult,
		Type: model.CommentTypeGeneral,
	}

	// Create comment on MR
	err = s.provider.CreateComment(ctx, request.ProjectID, request.MergeRequest.IID, comment)
	if err != nil {
		return errm.Wrap(err, "failed to create architecture review comment")
	}

	log.Info("architecture review created and posted")

	return nil
}

func (s *Reviewer) createCommentWithArchitectureReview(result *model.ArchitectureReviewResult) *model.Comment {
	reviewHeaders := prompts.DefaultLanguages[s.cfg.Language].ArchitectureReviewHeaders

	comment := strings.Builder{}
	comment.WriteString("## ")
	comment.WriteString(reviewHeaders.GeneralHeader)
	comment.WriteString("\n\n")

	// Add general overview if available
	if result.GeneralOverview != "" {
		comment.WriteString(result.GeneralOverview)
		comment.WriteString("\n\n")
	}

	// Add architecture issues
	if len(result.ArchitectureIssues) > 0 {
		comment.WriteString("### ")
		comment.WriteString(reviewHeaders.ArchitectureIssuesHeader)
		comment.WriteString("\n\n")
		for _, issue := range result.ArchitectureIssues {
			comment.WriteString("- **")
			comment.WriteString(issue.Title)
			comment.WriteString("**: ")
			comment.WriteString(issue.Description)
			if issue.Impact != "" {
				comment.WriteString(" ")
				comment.WriteString(issue.Impact)
			}
			if issue.Solution != "" {
				comment.WriteString(" ")
				comment.WriteString(issue.Solution)
			}
			comment.WriteString("\n")
		}
		comment.WriteString("\n")
	}

	// Add performance issues
	if len(result.PerformanceIssues) > 0 {
		comment.WriteString("### ")
		comment.WriteString(reviewHeaders.PerformanceIssuesHeader)
		comment.WriteString("\n\n")
		for _, issue := range result.PerformanceIssues {
			comment.WriteString("- **")
			comment.WriteString(issue.Title)
			comment.WriteString("**: ")
			comment.WriteString(issue.Description)
			if issue.Impact != "" {
				comment.WriteString(" ")
				comment.WriteString(issue.Impact)
			}
			if issue.Solution != "" {
				comment.WriteString(" ")
				comment.WriteString(issue.Solution)
			}
			comment.WriteString("\n")
		}
		comment.WriteString("\n")
	}

	// Add security issues
	if len(result.SecurityIssues) > 0 {
		comment.WriteString("### ")
		comment.WriteString(reviewHeaders.SecurityIssuesHeader)
		comment.WriteString("\n\n")
		for _, issue := range result.SecurityIssues {
			comment.WriteString("- **")
			comment.WriteString(issue.Title)
			comment.WriteString("**: ")
			comment.WriteString(issue.Description)
			if issue.Impact != "" {
				comment.WriteString(" ")
				comment.WriteString(issue.Impact)
			}
			if issue.Solution != "" {
				comment.WriteString(" ")
				comment.WriteString(issue.Solution)
			}
			comment.WriteString("\n")
		}
		comment.WriteString("\n")
	}

	// Add documentation needs
	if len(result.DocumentationNeeds) > 0 {
		comment.WriteString("### ")
		comment.WriteString(reviewHeaders.DocsImprovementHeader)
		comment.WriteString("\n\n")
		for _, issue := range result.DocumentationNeeds {
			comment.WriteString("- **")
			comment.WriteString(issue.Title)
			comment.WriteString("**: ")
			comment.WriteString(issue.Description)
			if issue.Impact != "" {
				comment.WriteString(" ")
				comment.WriteString(issue.Impact)
			}
			if issue.Solution != "" {
				comment.WriteString(" ")
				comment.WriteString(issue.Solution)
			}
			comment.WriteString("\n")
		}
		comment.WriteString("\n")
	}

	// If no findings at all, add a simple message
	if len(result.ArchitectureIssues) == 0 && len(result.PerformanceIssues) == 0 &&
		len(result.SecurityIssues) == 0 && len(result.DocumentationNeeds) == 0 &&
		result.GeneralOverview == "" {
		comment.WriteString("No significant architectural issues detected in this merge request.\n\n")
	}

	body := comment.String()

	return &model.Comment{
		Body: body,
		Type: model.CommentTypeGeneral,
	}
}
