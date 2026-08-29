package tensai

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestTensaiLLM(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "twsla-tensai-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dummyPath := filepath.Join(tempDir, "test.gguf")
	// Write minimal dummy file
	if err := os.WriteFile(dummyPath, []byte("GGUF\x03\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"), 0644); err != nil {
		t.Fatalf("failed to write dummy: %v", err)
	}

	llm, err := New(dummyPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx := context.Background()
	resp, err := llm.Call(ctx, "Log Details:\nSeverity: ERROR\nMessage: Connection refused")
	if err != nil {
		t.Logf("Call returned error as expected for dummy file: %v", err)
	} else {
		if resp == "" {
			t.Errorf("expected non-empty response")
		}
	}
}
