package model

import (
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
