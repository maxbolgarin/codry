package gitlab

type gitlabPayload struct {
	ObjectKind string `json:"object_kind"`
	EventType  string `json:"event_type"`
	User       struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
		Email    string `json:"email"`
	} `json:"user"`
	Project struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"project"`
	ObjectAttributes struct {
		IID          int    `json:"iid"`
		Action       string `json:"action"`
		State        string `json:"state"`
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
		URL          string `json:"url"`
		Title        string `json:"title"`
		Description  string `json:"description"`
		AuthorID     int    `json:"author_id"`
		LastCommit   struct {
			ID string `json:"id"`
		} `json:"last_commit"`
	} `json:"object_attributes"`
	// Reviewers information from webhook payload
	Reviewers []struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
		Email    string `json:"email"`
	} `json:"reviewers"`
	// Changes for tracking specific reviewer modifications
	Changes struct {
		Reviewers struct {
			Previous []struct {
				ID       int    `json:"id"`
				Username string `json:"username"`
				Name     string `json:"name"`
				Email    string `json:"email"`
			} `json:"previous"`
			Current []struct {
				ID       int    `json:"id"`
				Username string `json:"username"`
				Name     string `json:"name"`
				Email    string `json:"email"`
			} `json:"current"`
		} `json:"reviewers"`
	} `json:"changes"`
}
