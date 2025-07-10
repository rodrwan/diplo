package models

type DeployRequest struct {
	RepoURL string `json:"repo_url"`
	Name    string `json:"name,omitempty"`
}
