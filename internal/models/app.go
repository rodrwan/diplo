package models

type DeployRequest struct {
	RepoURL     string `json:"repo_url"`
	Name        string `json:"name,omitempty"`
	RuntimeType string `json:"runtime_type,omitempty"`
	Language    string `json:"language,omitempty"`
}
