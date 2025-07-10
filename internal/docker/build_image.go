package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/sirupsen/logrus"
)

// BuildImage builds a Docker image from a Dockerfile.
func (d *Client) BuildImage(imageName, dockerfileContent string) (string, error) {
	logrus.Infof("Building image: %s", imageName)
	d.sendDockerEvent("build_start", "Starting image build", map[string]interface{}{"image_name": imageName})

	buildCtx, err := d.createBuildContext(dockerfileContent)
	if err != nil {
		d.sendDockerEvent("build_error", "Error creating build context", map[string]interface{}{"error": err.Error()})
		return "", fmt.Errorf("error creating build context: %w", err)
	}

	// Construir imagen sin tag para evitar problemas de tagging
	buildOptions := types.ImageBuildOptions{
		Dockerfile:  dockerfileName,
		Remove:      true,
		ForceRemove: true,
		NoCache:     true,
		// No incluir Tags aquí para evitar problemas
	}

	d.sendDockerEvent("build_step", "Building Docker image", map[string]interface{}{"step": "docker_build", "image_name": imageName})
	buildResp, err := d.cli.ImageBuild(context.Background(), buildCtx, buildOptions)
	if err != nil {
		d.sendDockerEvent("build_error", "Error building image", map[string]interface{}{"error": err.Error()})
		return "", fmt.Errorf("error building image: %w", err)
	}
	defer buildResp.Body.Close()

	// Capturar el ID de imagen del stream de build
	var imageID string
	if err := d.streamBuildOutputWithID(buildResp.Body, &imageID); err != nil {
		d.sendDockerEvent("build_error", "Image build failed", map[string]interface{}{"error": err.Error()})
		return "", err
	}

	if imageID == "" {
		d.sendDockerEvent("build_error", "No image ID captured from build output", nil)
		return "", fmt.Errorf("no image ID captured from build output")
	}

	d.sendDockerEvent("build_step", "Tagging built image", map[string]interface{}{
		"step":     "tag_image",
		"image_id": imageID,
		"tag":      imageName,
	})

	// Asignar tag manualmente después del build
	if err := d.cli.ImageTag(context.Background(), imageID, imageName); err != nil {
		d.sendDockerEvent("build_error", "Error tagging image", map[string]interface{}{
			"error":    err.Error(),
			"image_id": imageID,
			"tag":      imageName,
		})
		return "", fmt.Errorf("error tagging image %s with tag %s: %w", imageID, imageName, err)
	}

	// Verificar que el tag se asignó correctamente
	d.sendDockerEvent("build_step", "Verifying tagged image", map[string]interface{}{"step": "verify_tag", "tag": imageName})
	taggedImageID, err := d.findImageByTag(imageName)
	if err != nil {
		logrus.Warnf("Tag verification failed, using original image ID: %s", imageID)
		d.sendDockerEvent("build_warning", "Tag verification failed, using original image ID", map[string]interface{}{
			"image_id": imageID,
			"tag":      imageName,
		})
		// Continuar con el imageID original si la verificación falla
		taggedImageID = imageID
	}

	d.sendDockerEvent("build_success", "Image built and tagged successfully", map[string]interface{}{
		"image_name": imageName,
		"image_id":   taggedImageID,
	})
	logrus.Infof("Image built and tagged successfully: %s (ID: %s)", imageName, taggedImageID)
	return taggedImageID, nil
}

// createBuildContext creates a tar archive containing the Dockerfile.
func (d *Client) createBuildContext(dockerfileContent string) (*bytes.Buffer, error) {
	if strings.TrimSpace(dockerfileContent) == "" {
		return nil, fmt.Errorf("dockerfile is empty")
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	defer tw.Close()

	header := &tar.Header{
		Name: dockerfileName,
		Mode: 0644,
		Size: int64(len(dockerfileContent)),
	}

	if err := tw.WriteHeader(header); err != nil {
		return nil, fmt.Errorf("error writing tar header: %w", err)
	}
	if _, err := tw.Write([]byte(dockerfileContent)); err != nil {
		return nil, fmt.Errorf("error writing dockerfile to tar: %w", err)
	}

	return &buf, nil
}

// streamBuildOutputWithID processes the streaming output from an image build and captures the final image ID.
func (d *Client) streamBuildOutputWithID(reader io.Reader, imageID *string) error {
	d.sendDockerEvent("build_step", "Streaming build logs", map[string]interface{}{"step": "stream_logs"})
	decoder := json.NewDecoder(reader)
	for {
		var jsonMessage jsonmessage.JSONMessage
		if err := decoder.Decode(&jsonMessage); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("error decoding build output: %w", err)
		}

		if jsonMessage.Stream != "" {
			logMessage := strings.TrimSpace(jsonMessage.Stream)
			logrus.Debug(logMessage)
			d.sendDockerEvent("build_log", logMessage, nil)

			// Capturar el ID de imagen de diferentes formatos de output
			if strings.Contains(logMessage, "Successfully built") {
				parts := strings.Fields(logMessage)
				if len(parts) >= 3 {
					*imageID = parts[2]
					logrus.Debugf("Captured image ID from 'Successfully built': %s", *imageID)
				}
			} else if strings.HasPrefix(logMessage, "sha256:") {
				// A veces el ID viene como "sha256:xxxxx"
				*imageID = strings.TrimPrefix(logMessage, "sha256:")
				logrus.Debugf("Captured image ID from 'sha256:': %s", *imageID)
			}
		}

		// Capturar ID de imagen desde el campo Aux si está disponible
		if jsonMessage.Aux != nil {
			var auxData map[string]interface{}
			if auxBytes, err := json.Marshal(jsonMessage.Aux); err == nil {
				if err := json.Unmarshal(auxBytes, &auxData); err == nil {
					if id, exists := auxData["ID"]; exists {
						if idStr, ok := id.(string); ok && idStr != "" {
							*imageID = idStr
							logrus.Debugf("Captured image ID from Aux field: %s", *imageID)
						}
					}
				}
			}
		}

		if jsonMessage.Error != nil {
			return fmt.Errorf("build failed: %s", jsonMessage.Error.Message)
		}
	}

	if *imageID == "" {
		return fmt.Errorf("no image ID found in build output")
	}

	return nil
}

// findImageByTagWithRetry finds an image ID by its tag with retry logic.
func (d *Client) findImageByTagWithRetry(imageName string, maxRetries int, retryDelay time.Duration) (string, error) {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		logrus.Debugf("Attempt %d/%d: Searching for image %s", attempt, maxRetries, imageName)

		imageID, err := d.findImageByTag(imageName)
		if err == nil {
			logrus.Debugf("Found image %s on attempt %d with ID: %s", imageName, attempt, imageID)
			return imageID, nil
		}

		lastErr = err
		if attempt < maxRetries {
			logrus.Debugf("Image not found on attempt %d, retrying in %v...", attempt, retryDelay)
			time.Sleep(retryDelay)
		}
	}

	return "", fmt.Errorf("failed to find image %s after %d attempts: %w", imageName, maxRetries, lastErr)
}

// findImageByTag finds an image ID by its tag.
func (d *Client) findImageByTag(imageName string) (string, error) {
	images, err := d.cli.ImageList(context.Background(), types.ImageListOptions{})
	if err != nil {
		return "", fmt.Errorf("error listing images: %w", err)
	}

	logrus.Debugf("Searching for image %s among %d images", imageName, len(images))

	for _, image := range images {
		logrus.Debugf("Checking image ID: %s, Tags: %v", image.ID, image.RepoTags)
		for _, tag := range image.RepoTags {
			if tag == imageName {
				logrus.Debugf("Found matching tag %s for image ID %s", tag, image.ID)
				return image.ID, nil
			}
		}
	}

	// Log all available images for debugging
	logrus.Debugf("Available images:")
	for _, image := range images {
		logrus.Debugf("  ID: %s, Tags: %v", image.ID, image.RepoTags)
	}

	return "", fmt.Errorf("image %s not found", imageName)
}
