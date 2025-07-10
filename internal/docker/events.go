package docker

import "time"

// DockerEvent represents an event from the Docker process.
type DockerEvent struct {
	Type    string                 `json:"type"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Time    time.Time              `json:"time"`
}

// sendDockerEvent sends a Docker event if a callback is configured.
func (d *Client) sendDockerEvent(eventType, message string, data map[string]interface{}) {
	if d.eventCallback != nil {
		event := DockerEvent{
			Type:    eventType,
			Message: message,
			Data:    data,
			Time:    time.Now(),
		}
		d.eventCallback(event)
	}
}
