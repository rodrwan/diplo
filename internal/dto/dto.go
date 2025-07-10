package dto

type App struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	RepoUrl     string `json:"repo_url"`
	Language    string `json:"language"`
	Port        int    `json:"port"`
	ContainerID string `json:"container_id"`
	ImageID     string `json:"image_id"`
	Status      string `json:"status"`
	ErrorMsg    string `json:"error_msg"`
}
