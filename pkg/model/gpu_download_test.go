package model

import (
	"runtime"
	"testing"
)

func TestGetWGPUAssetInfo(t *testing.T) {
	url, libFile, err := GetWGPUAssetInfo()
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("skipping on non-mainstream OS")
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url == "" || libFile == "" {
		t.Errorf("expected non-empty url and libFile, got url=%q, libFile=%q", url, libFile)
	}
	t.Logf("Platform: %s/%s -> URL: %s, File: %s", runtime.GOOS, runtime.GOARCH, url, libFile)
}

func TestDefaultLibDir(t *testing.T) {
	dir := DefaultLibDir()
	if dir == "" {
		t.Errorf("expected non-empty DefaultLibDir")
	}
	t.Logf("DefaultLibDir: %s", dir)
}
