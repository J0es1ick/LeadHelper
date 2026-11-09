package domain

type JiraUser struct {
	ID    string `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

type JiraTeamStatus struct {
	Scope   string   `json:"scope"`
	Site    string   `json:"site"`
	Members []Member `json:"members"`
	Linked  int      `json:"linked"`
}
