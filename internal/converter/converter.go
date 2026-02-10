package converter

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
)

// Convert runs ebook-convert to produce a .kepub.epub file in convertedDir.
// Returns the path to the converted file.
func Convert(ctx context.Context, inputPath, convertedDir string) (string, error) {
	baseName := filepath.Base(inputPath)
	ext := filepath.Ext(baseName)
	newBaseName := strings.TrimSuffix(baseName, ext) + ".kepub.epub"
	outputPath := filepath.Join(convertedDir, newBaseName)

	slog.Info("Starting conversion with ebook-convert", "input", inputPath, "output", outputPath)

	cmd := exec.CommandContext(ctx, "ebook-convert", inputPath, outputPath)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ebook-convert failed: %v\nStderr: %s", err, stderr.String())
	}

	slog.Info("ebook-convert completed successfully")
	return outputPath, nil
}
