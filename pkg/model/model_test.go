package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListAndFindModels(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "twsla-model-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create dummy gguf file
	dummyFile := filepath.Join(tempDir, "test-model.gguf")
	if err := os.WriteFile(dummyFile, []byte("GGUF_TEST"), 0644); err != nil {
		t.Fatalf("failed to write dummy model: %v", err)
	}

	models, err := ListModels(tempDir)
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0].Name != "test-model.gguf" {
		t.Errorf("expected model name test-model.gguf, got %s", models[0].Name)
	}

	found, err := FindModel(tempDir, "test-model")
	if err != nil {
		t.Fatalf("FindModel failed: %v", err)
	}
	if found != dummyFile {
		t.Errorf("expected %s, got %s", dummyFile, found)
	}

	// Test preset names
	presets := GetPresetNames()
	if len(presets) == 0 {
		t.Errorf("expected presets, got empty list")
	}
}
