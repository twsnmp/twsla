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

	// Create dummy preset model file
	presetFile := filepath.Join(tempDir, "qwen2.5-0.5b-instruct-q8_0.gguf")
	if err := os.WriteFile(presetFile, []byte("GGUF_QWEN"), 0644); err != nil {
		t.Fatalf("failed to write dummy preset model: %v", err)
	}

	// Test finding model by preset name
	foundPreset, err := FindModel(tempDir, "qwen2.5-0.5b")
	if err != nil {
		t.Fatalf("FindModel with preset name failed: %v", err)
	}
	if foundPreset != presetFile {
		t.Errorf("expected %s, got %s", presetFile, foundPreset)
	}

	// Test finding model by partial/prefix name
	foundPartial, err := FindModel(tempDir, "qwen2.5-0.5b-instruct")
	if err != nil {
		t.Fatalf("FindModel with partial name failed: %v", err)
	}
	if foundPartial != presetFile {
		t.Errorf("expected %s, got %s", presetFile, foundPartial)
	}

	// Test RemoveModel and .cache cleanup
	cacheFile := dummyFile + ".cache"
	if err := os.WriteFile(cacheFile, []byte("dummy-cache-data"), 0644); err != nil {
		t.Fatalf("failed to write dummy cache: %v", err)
	}

	if err := RemoveModel(tempDir, "test-model.gguf"); err != nil {
		t.Fatalf("RemoveModel failed: %v", err)
	}
	if _, err := os.Stat(dummyFile); !os.IsNotExist(err) {
		t.Errorf("expected dummyFile to be removed")
	}
	if _, err := os.Stat(cacheFile); !os.IsNotExist(err) {
		t.Errorf("expected cacheFile to be removed")
	}
}


