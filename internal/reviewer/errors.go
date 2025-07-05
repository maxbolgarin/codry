package reviewer

import "github.com/maxbolgarin/errm"

var (
	errAlreadyHasAIDescription = errm.New("already has AI description")
	errEmptyDescription        = errm.New("empty description")
)
