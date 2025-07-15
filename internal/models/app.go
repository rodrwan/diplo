package models

type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type DeployRequest struct {
	RepoURL     string   `json:"repo_url"`
	Name        string   `json:"name,omitempty"`
	RuntimeType string   `json:"runtime_type,omitempty"`
	Language    string   `json:"language,omitempty"`
	EnvVars     []EnvVar `json:"env_vars,omitempty"`
	GitHubToken string   `json:"github_token,omitempty"`
}
