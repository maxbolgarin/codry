package model

import (
	"strconv"
	"time"
)

type ReviewType string

const (
	ReviewTypeDescription   ReviewType = "description"
	ReviewTypeCodeReview    ReviewType = "code"
	ReviewTypeListOfChanges ReviewType = "list_of_changes"
	ReviewTypeArchitecture  ReviewType = "architecture"
)

// CodeEvent represents a webhook event from any provider
type CodeEvent struct {
	Type         string
	Action       string
	ProjectID    string
	MergeRequest *MergeRequest
	Comment      *Comment
	User         *User
	Timestamp    time.Time
}

// ReviewRequest represents a code review request
type ReviewRequest struct {
	ProjectID    string
	MergeRequest *MergeRequest
	Changes      []*FileDiff
}

func (r ReviewRequest) String() string {
	return r.ProjectID + ":" + r.MergeRequest.SHA + ":" + strconv.Itoa(r.MergeRequest.IID)
}

// ReviewResult represents the result of a code review process
type ReviewResult struct {
	Success                   bool
	ProcessedFiles            int
	CommentsCreated           int
	DescriptionUpdated        bool
	ChangesOverviewUpdated    bool
	ArchitectureReviewUpdated bool
	Errors                    []error
}

// ArchitectureReviewResult represents the result of an architecture review
type ArchitectureReviewResult struct {
	GeneralOverview    string                      `json:"general_overview,omitempty"`
	ArchitectureIssues []ArchitectureReviewFinding `json:"architecture_issues,omitempty"`
	PerformanceIssues  []ArchitectureReviewFinding `json:"performance_issues,omitempty"`
	SecurityIssues     []ArchitectureReviewFinding `json:"security_issues,omitempty"`
	DocumentationNeeds []ArchitectureReviewFinding `json:"documentation_needs,omitempty"`
}

// ArchitectureReviewFinding represents a single finding from the architecture review
type ArchitectureReviewFinding struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Impact      string `json:"impact"`
	Solution    string `json:"solution"`
}
