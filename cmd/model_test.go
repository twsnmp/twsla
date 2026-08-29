package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestModelCommands(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "twsla-cmd-model-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	modelDirFlag = tempDir

	// Test list when empty
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"model", "list", "--modelDir", tempDir})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("model list failed: %v", err)
	}

	// Create dummy file
	dummyPath := filepath.Join(tempDir, "sample.gguf")
	if err := os.WriteFile(dummyPath, []byte("dummy"), 0644); err != nil {
		t.Fatalf("failed to write dummy: %v", err)
	}

	// Test list with file
	rootCmd.SetArgs([]string{"model", "list", "--modelDir", tempDir})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("model list failed: %v", err)
	}

	// Test presets
	rootCmd.SetArgs([]string{"model", "presets"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("model presets failed: %v", err)
	}

	// Test remove
	rootCmd.SetArgs([]string{"model", "remove", "sample.gguf", "--modelDir", tempDir})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("model remove failed: %v", err)
	}
}
