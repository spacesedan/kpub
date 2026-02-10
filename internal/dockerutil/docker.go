package dockerutil

import (
	"bufio"
	"fmt"
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

// PullImage pulls a Docker image.
// Each line of output is sent to the output channel (if non-nil).
func PullImage(image string, output chan<- string) error {
	cmd := exec.Command("docker", "pull", "--platform", "linux/amd64", image)
	return runStreaming(cmd, output)
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

// runStreaming runs a command and streams its combined stdout/stderr
// line-by-line to the output channel.
func runStreaming(cmd *exec.Cmd, output chan<- string) error {
	// Use a pipe to combine stdout and stderr.
	pr, pw, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("creating pipe: %w", err)
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return fmt.Errorf("starting command: %w", err)
	}
	// Close the write end in the parent so the scanner sees EOF when the child exits.
	pw.Close()

	scanner := bufio.NewScanner(pr)
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)
	// Docker pull uses \r to overwrite progress in-place. Split on both
	// \r and \n so we see intermediate progress (e.g. "Downloading 12MB/45MB").
	scanner.Split(scanCRLF)
	var lastLines []string
	for scanner.Scan() {
		line := scanner.Text()
		lastLines = appendCapped(lastLines, line, 10)
		if output != nil {
			output <- line
		}
	}
	pr.Close()

	if err := cmd.Wait(); err != nil {
		tail := strings.Join(lastLines, "\n")
		return fmt.Errorf("%w\n%s", err, tail)
	}
	return nil
}

// scanCRLF is a bufio.SplitFunc that splits on \n, \r\n, or bare \r.
// This lets us capture Docker's carriage-return progress updates.
func scanCRLF(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			return i + 1, data[:i], nil
		}
		if data[i] == '\r' {
			// \r\n counts as one line break.
			if i+1 < len(data) && data[i+1] == '\n' {
				return i + 2, data[:i], nil
			}
			return i + 1, data[:i], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// appendCapped appends s to lines, keeping at most n entries.
func appendCapped(lines []string, s string, n int) []string {
	lines = append(lines, s)
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}
