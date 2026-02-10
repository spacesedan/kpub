package dockerutil

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

// CheckDocker verifies that the docker CLI is available on the PATH.
func CheckDocker() error {
	_, err := exec.LookPath("docker")
	if err != nil {
		return fmt.Errorf("docker not found in PATH: %w", err)
	}
	return nil
}

// RemoveContainer force-removes a container by name, ignoring "not found" errors.
func RemoveContainer(name string) error {
	cmd := exec.Command("docker", "rm", "-f", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "No such container") {
			return nil
		}
		return fmt.Errorf("removing container %q: %s", name, strings.TrimSpace(string(out)))
	}
	return nil
}

// StopContainer gracefully stops a container by name using SIGTERM, then removes it.
// Returns nil if the container does not exist.
func StopContainer(name string) error {
	cmd := exec.Command("docker", "stop", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "No such container") {
			return nil
		}
		return fmt.Errorf("stopping container %q: %s", name, strings.TrimSpace(string(out)))
	}
	return RemoveContainer(name)
}

// PullImage pulls a Docker image via the Docker Engine API, streaming
// progress to the output channel as human-readable lines like
// "Downloading  120.5 MB / 557.3 MB".
func PullImage(image string, output chan<- string) error {
	name, tag := parseImageRef(image)

	sock := dockerSocket()
	httpc := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", sock)
			},
		},
	}

	params := url.Values{}
	params.Set("fromImage", name)
	params.Set("tag", tag)
	params.Set("platform", "linux/amd64")

	resp, err := httpc.Post("http://localhost/v1.41/images/create?"+params.Encode(), "", nil)
	if err != nil {
		return fmt.Errorf("pull request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pull failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	tracker := &pullTracker{
		layers: make(map[string]*layerProgress),
	}

	decoder := json.NewDecoder(resp.Body)
	for decoder.More() {
		var evt pullEvent
		if err := decoder.Decode(&evt); err != nil {
			break
		}
		if evt.Error != "" {
			return fmt.Errorf("pull: %s", evt.Error)
		}
		if output != nil {
			tracker.update(evt)
			output <- tracker.render()
		}
	}
	return nil
}

// pullEvent represents a single JSON event from the Docker pull stream.
type pullEvent struct {
	Status         string         `json:"status"`
	ID             string         `json:"id"`
	Progress       string         `json:"progress"`
	ProgressDetail progressDetail `json:"progressDetail"`
	Error          string         `json:"error"`
}

type progressDetail struct {
	Current int64 `json:"current"`
	Total   int64 `json:"total"`
}

type layerProgress struct {
	status  string
	current int64
	total   int64
}

// pullTracker maintains per-layer state and renders a Docker-style view.
type pullTracker struct {
	ids    []string // insertion order
	layers map[string]*layerProgress
	header string // top-level status like "Pulling from ..."
}

func (t *pullTracker) update(evt pullEvent) {
	if evt.ID == "" {
		// Top-level status lines.
		if evt.Status != "" {
			t.header = evt.Status
		}
		return
	}

	lp, ok := t.layers[evt.ID]
	if !ok {
		lp = &layerProgress{}
		t.layers[evt.ID] = lp
		t.ids = append(t.ids, evt.ID)
	}

	lp.status = evt.Status
	lp.current = evt.ProgressDetail.Current
	lp.total = evt.ProgressDetail.Total
}

func (t *pullTracker) render() string {
	var b strings.Builder

	if t.header != "" {
		b.WriteString(t.header)
		b.WriteByte('\n')
	}

	var totalBytes, currentBytes int64

	for _, id := range t.ids {
		lp := t.layers[id]
		short := id
		if len(short) > 12 {
			short = short[:12]
		}

		switch strings.ToLower(lp.status) {
		case "downloading":
			if lp.total > 0 {
				pct := float64(lp.current) / float64(lp.total) * 100
				fmt.Fprintf(&b, "%s: Downloading  %.1f / %.1f MB  (%.0f%%)\n",
					short,
					float64(lp.current)/1e6,
					float64(lp.total)/1e6,
					pct)
				totalBytes += lp.total
				currentBytes += lp.current
			} else {
				fmt.Fprintf(&b, "%s: Downloading\n", short)
			}
		case "extracting":
			if lp.total > 0 {
				pct := float64(lp.current) / float64(lp.total) * 100
				fmt.Fprintf(&b, "%s: Extracting   %.1f / %.1f MB  (%.0f%%)\n",
					short,
					float64(lp.current)/1e6,
					float64(lp.total)/1e6,
					pct)
			} else {
				fmt.Fprintf(&b, "%s: Extracting\n", short)
			}
		default:
			fmt.Fprintf(&b, "%s: %s\n", short, lp.status)
		}
	}

	// Aggregate download summary at the bottom.
	if totalBytes > 0 {
		pct := float64(currentBytes) / float64(totalBytes) * 100
		fmt.Fprintf(&b, "Total: %.1f / %.1f MB  (%.0f%%)",
			float64(currentBytes)/1e6, float64(totalBytes)/1e6, pct)
	}

	return b.String()
}

// parseImageRef splits "ghcr.io/spacesedan/kpub:latest" into name and tag.
func parseImageRef(image string) (name, tag string) {
	if i := strings.LastIndex(image, ":"); i > 0 {
		afterColon := image[i+1:]
		if !strings.Contains(afterColon, "/") {
			return image[:i], afterColon
		}
	}
	return image, "latest"
}

// dockerSocket returns the path to the Docker daemon Unix socket.
func dockerSocket() string {
	if h := os.Getenv("DOCKER_HOST"); h != "" {
		if strings.HasPrefix(h, "unix://") {
			return strings.TrimPrefix(h, "unix://")
		}
	}
	return "/var/run/docker.sock"
}

// RunContainer starts a container with the given name, image, and data directory bind mount.
// If detach is true, the container runs in the background (output suppressed).
// If foreground, stdout/stderr/stdin are attached to the terminal.
func RunContainer(name, image, dataDir string, detach bool) error {
	args := []string{"run", "--platform", "linux/amd64", "--name", name}
	if detach {
		args = append(args, "-d")
	} else {
		args = append(args, "-it")
	}
	args = append(args, "-v", dataDir+":/data", image)

	cmd := exec.Command("docker", args...)
	if detach {
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("running container %q: %s", name, strings.TrimSpace(string(out)))
		}
		return nil
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running container %q: %w", name, err)
	}
	return nil
}
