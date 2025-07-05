package model

import (
	"strconv"
	"time"
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
	Success            bool
	ProcessedFiles     int
	CommentsCreated    int
	DescriptionUpdated bool
	Errors             []error
}
