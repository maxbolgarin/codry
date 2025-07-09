package bitbucket

import "encoding/json"

// Bitbucket API structures
type bitbucketUser struct {
	UUID        string `json:"uuid"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Nickname    string `json:"nickname"`
	Links       struct {
		Self struct {
			Href string `json:"href"`
		} `json:"self"`
		Avatar struct {
			Href string `json:"href"`
		} `json:"avatar"`
	} `json:"links"`
}

type bitbucketPullRequest struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	CreatedOn   string `json:"created_on"`
	UpdatedOn   string `json:"updated_on"`
	Source      struct {
		Branch struct {
			Name string `json:"name"`
		} `json:"branch"`
		Commit struct {
			Hash string `json:"hash"`
		} `json:"commit"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	} `json:"source"`
	Destination struct {
		Branch struct {
			Name string `json:"name"`
		} `json:"branch"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	} `json:"destination"`
	Author    bitbucketUser `json:"author"`
	Reviewers []struct {
		User bitbucketUser `json:"user"`
	} `json:"reviewers"`
	Links struct {
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
		Diff struct {
			Href string `json:"href"`
		} `json:"diff"`
		Comments struct {
			Href string `json:"href"`
		} `json:"comments"`
	} `json:"links"`
}

type bitbucketRepository struct {
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	FullName  string `json:"full_name"`
	Workspace struct {
		Slug string `json:"slug"`
	} `json:"workspace"`
}

type bitbucketComment struct {
	ID        int    `json:"id"`
	CreatedOn string `json:"created_on"`
	UpdatedOn string `json:"updated_on"`
	Content   struct {
		Raw    string `json:"raw"`
		Markup string `json:"markup"`
		HTML   string `json:"html"`
	} `json:"content"`
	User   bitbucketUser `json:"user"`
	Inline struct {
		Path string `json:"path"`
		From int    `json:"from"`
		To   int    `json:"to"`
	} `json:"inline"`
	Links struct {
		Self struct {
			Href string `json:"href"`
		} `json:"self"`
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
	} `json:"links"`
}

type bitbucketPayload struct {
	Repository  bitbucketRepository  `json:"repository"`
	PullRequest bitbucketPullRequest `json:"pullrequest"`
	Actor       bitbucketUser        `json:"actor"`
	EventKey    string               `json:"eventKey,omitempty"` // Alternative field name
	Type        string               `json:"type,omitempty"`     // Some events use this
	Changes     json.RawMessage      `json:"changes,omitempty"`  // For update events
	Approval    json.RawMessage      `json:"approval,omitempty"` // For approval events
	Comment     json.RawMessage      `json:"comment,omitempty"`  // For comment events
}

type bitbucketCommit struct {
	Hash    string `json:"hash"`
	Date    string `json:"date"`
	Message string `json:"message"`
	Author  struct {
		Raw         string `json:"raw"`
		UUID        string `json:"uuid"`
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
	} `json:"author"`
	Links struct {
		Self struct {
			Href string `json:"href"`
		} `json:"self"`
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
	} `json:"links"`
}
