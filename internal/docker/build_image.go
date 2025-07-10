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

	buildOptions := types.ImageBuildOptions{
		Tags:        []string{imageName},
		Dockerfile:  dockerfileName,
		Remove:      true,
		ForceRemove: true,
		NoCache:     true,
	}

	d.sendDockerEvent("build_step", "Building Docker image", map[string]interface{}{"step": "docker_build", "image_name": imageName})
	buildResp, err := d.cli.ImageBuild(context.Background(), buildCtx, buildOptions)
	if err != nil {
		d.sendDockerEvent("build_error", "Error building image", map[string]interface{}{"error": err.Error()})
		return "", fmt.Errorf("error building image: %w", err)
	}
	defer buildResp.Body.Close()

	// Capturar el último ID de imagen del stream de build
	var lastImageID string
	if err := d.streamBuildOutputWithID(buildResp.Body, &lastImageID); err != nil {
		d.sendDockerEvent("build_error", "Image build failed", map[string]interface{}{"error": err.Error()})
		return "", err
	}

	d.sendDockerEvent("build_step", "Searching for built image", map[string]interface{}{"step": "find_image", "image_name": imageName})

	// Intentar encontrar la imagen con retry logic
	imageID, err := d.findImageByTagWithRetry(imageName, 5, time.Second*2)
	if err != nil {
		// Si no encontramos por tag, usar el ID capturado del build output como fallback
		if lastImageID != "" {
			logrus.Warnf("No se pudo encontrar imagen por tag %s, usando ID del build: %s", imageName, lastImageID)
			d.sendDockerEvent("build_warning", "Using fallback image ID from build output", map[string]interface{}{
				"image_name": imageName,
				"image_id":   lastImageID,
			})
			imageID = lastImageID
		} else {
			d.sendDockerEvent("build_error", "Image not found after build", map[string]interface{}{"error": err.Error()})
			return "", fmt.Errorf("image not found after build: %w", err)
		}
	}

	d.sendDockerEvent("build_success", "Image built successfully", map[string]interface{}{"image_name": imageName, "image_id": imageID})
	logrus.Infof("Image built successfully: %s (ID: %s)", imageName, imageID)
	return imageID, nil
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
func (d *Client) streamBuildOutputWithID(reader io.Reader, lastImageID *string) error {
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

			// Capturar el ID de imagen del output final
			if strings.Contains(logMessage, "Successfully built") {
				parts := strings.Fields(logMessage)
				if len(parts) >= 3 {
					*lastImageID = parts[2]
					logrus.Debugf("Captured image ID from build output: %s", *lastImageID)
				}
			}
		}

		if jsonMessage.Error != nil {
			return fmt.Errorf("build failed: %s", jsonMessage.Error.Message)
		}
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
