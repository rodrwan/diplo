package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"
)

// Constants for magic values and default settings.
const (
	defaultHostIP      = "0.0.0.0"
	defaultStopTimeout = 10 * time.Second
	dockerfileName     = "Dockerfile"
)

// DockerEventCallback is a function called when a Docker event occurs.
type DockerEventCallback func(event DockerEvent)

// Client manages interactions with the Docker daemon.
type Client struct {
	cli           *client.Client
	eventCallback DockerEventCallback
}

// NewClient creates a new Docker client.
func NewClient() (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("error creating Docker client: %w", err)
	}
	return &Client{cli: cli}, nil
}

// SetEventCallback sets the callback for Docker events.
func (d *Client) SetEventCallback(callback DockerEventCallback) {
	d.eventCallback = callback
}

// GetEventCallback gets the current Docker event callback.
func (d *Client) GetEventCallback() DockerEventCallback {
	return d.eventCallback
}

// StopContainer stops and removes a container.
func (d *Client) StopContainer(containerID string) error {
	logrus.Infof("Stopping container: %s", containerID)
	timeout := int(defaultStopTimeout.Seconds())
	if err := d.cli.ContainerStop(context.Background(), containerID, container.StopOptions{Timeout: &timeout}); err != nil {
		return fmt.Errorf("error stopping container: %w", err)
	}

	if err := d.cli.ContainerRemove(context.Background(), containerID, types.ContainerRemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("error removing container: %w", err)
	}

	logrus.Infof("Container stopped and removed: %s", containerID)
	return nil
}

// GetContainerStatus returns the status of a container.
func (d *Client) GetContainerStatus(containerID string) (string, error) {
	containerJSON, err := d.cli.ContainerInspect(context.Background(), containerID)
	if err != nil {
		return "", fmt.Errorf("error inspecting container: %w", err)
	}

	return containerJSON.State.Status, nil
}

// GetContainerLogsStream gets a real-time stream of container logs.
func (d *Client) GetContainerLogsStream(containerID string) (io.ReadCloser, error) {
	logOptions := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       "100",
	}
	logs, err := d.cli.ContainerLogs(context.Background(), containerID, logOptions)
	if err != nil {
		return nil, fmt.Errorf("error getting container logs: %w", err)
	}
	return logs, nil
}

// Close closes the Docker client connection.
func (d *Client) Close() error {
	return d.cli.Close()
}

// GetLastCommitHash gets the latest commit hash from a Git repository.
func (d *Client) GetLastCommitHash(repoURL string) (string, error) {
	tempDir, err := os.MkdirTemp("", "diplo-git-*")
	if err != nil {
		return "", fmt.Errorf("error creating temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	cloneCmd := exec.Command("git", "clone", "--depth", "1", repoURL, tempDir)
	if err := cloneCmd.Run(); err != nil {
		return "", fmt.Errorf("error cloning repo: %w", err)
	}

	hashCmd := exec.Command("git", "rev-parse", "HEAD")
	hashCmd.Dir = tempDir
	hashOutput, err := hashCmd.Output()
	if err != nil {
		return "", fmt.Errorf("error getting commit hash: %w", err)
	}

	hash := strings.TrimSpace(string(hashOutput))
	if hash == "" {
		hash = fmt.Sprintf("commit_%d", time.Now().Unix())
	}

	logrus.Debugf("Latest commit hash for %s: %s", repoURL, hash)
	return hash, nil
}

// GenerateImageTag creates a unique image tag based on the app ID and commit hash.
func (d *Client) GenerateImageTag(appID, repoURL string) (string, error) {
	d.sendDockerEvent("tag_start", "Generating unique image tag", map[string]interface{}{"app_id": appID, "repo_url": repoURL})

	hash, err := d.GetLastCommitHash(repoURL)
	if err != nil {
		logrus.Warnf("Error getting commit hash, using fallback: %v", err)
		d.sendDockerEvent("tag_warning", "Error getting commit hash, using fallback", map[string]interface{}{"warning": err.Error()})
		hash = fmt.Sprintf("fallback_%d", time.Now().Unix())
	} else {
		d.sendDockerEvent("tag_step", "Commit hash obtained", map[string]interface{}{"step": "get_commit_hash", "hash": hash})
	}

	// Limpiar el appID removiendo el prefijo 'app_' si existe
	cleanAppID := strings.TrimPrefix(appID, "app_")

	// Crear un tag más limpio: diplo-{cleanAppID}-{hash8}
	// Usar guiones en lugar de guiones bajos para mejor compatibilidad
	tag := fmt.Sprintf("diplo-%s-%s", cleanAppID, hash[:8])

	// Reemplazar cualquier guión bajo restante con guiones
	tag = strings.ReplaceAll(tag, "_", "-")

	// Convertir a minúsculas para asegurar compatibilidad con Docker
	tag = strings.ToLower(tag)

	d.sendDockerEvent("tag_success", "Tag generated successfully", map[string]interface{}{"tag": tag, "hash": hash})
	logrus.Infof("Generated image tag: %s", tag)
	return tag, nil
}

// CleanupOldImages cleans up old images for a specific application.
func (d *Client) CleanupOldImages(appID string, keepCount int) error {
	logrus.Infof("Cleaning up old images for app: %s (keeping %d)", appID, keepCount)

	images, err := d.cli.ImageList(context.Background(), types.ImageListOptions{})
	if err != nil {
		return fmt.Errorf("error listing images: %w", err)
	}

	var appImages []types.ImageSummary
	// Limpiar appID y crear prefijo compatible con el nuevo formato
	cleanAppID := strings.TrimPrefix(appID, "app_")
	cleanAppID = strings.ReplaceAll(cleanAppID, "_", "-")
	cleanAppID = strings.ToLower(cleanAppID)
	prefix := fmt.Sprintf("diplo-%s-", cleanAppID)

	for _, img := range images {
		for _, tag := range img.RepoTags {
			if strings.HasPrefix(tag, prefix) {
				appImages = append(appImages, img)
				break
			}
		}
	}

	removedCount := 0
	if len(appImages) > keepCount {
		imagesToRemove := appImages[keepCount:]
		for _, img := range imagesToRemove {
			logrus.Infof("Removing old image: %s", img.ID)
			_, errs := d.cli.ImageRemove(context.Background(), img.ID, types.ImageRemoveOptions{Force: true})
			if errs != nil {
				logrus.Warnf("Error removing image %s: %v", img.ID, errs)
			} else {
				removedCount++
			}
		}
	}

	// Si se removieron imágenes, limpiar dangling images automáticamente
	if removedCount > 0 {
		logrus.Debugf("Removed %d images for app %s, cleaning dangling images...", removedCount, appID)
		if err := d.PruneDanglingImages(); err != nil {
			logrus.Warnf("Error pruning dangling images after cleanup: %v", err)
		}
	}

	return nil
}

// PruneDanglingImages removes all dangling (<none>) images.
func (d *Client) PruneDanglingImages() error {
	logrus.Infof("Pruning dangling images (<none>)...")
	pruneFilters := filters.NewArgs()
	pruneFilters.Add("dangling", "true")
	report, err := d.cli.ImagesPrune(context.Background(), pruneFilters)
	if err != nil {
		return fmt.Errorf("error pruning dangling images: %w", err)
	}

	if len(report.ImagesDeleted) > 0 {
		logrus.Infof("Pruned %d dangling images, reclaimed %d bytes", len(report.ImagesDeleted), report.SpaceReclaimed)
	} else {
		logrus.Infof("No dangling images to prune.")
	}
	return nil
}
