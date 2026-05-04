package cmd

import (
	"reflect"
	"testing"
)

func TestAnomalyModes(t *testing.T) {
	// 非常にシンプルなロジックテストの例
	// 本来はanomalyMain内の正規表現マッチングなどをテストすべきですが、
	// ここではモード設定の妥当性などをチェックする構造だけ示します。
	modes := []string{"tfidf", "sql", "os", "dir", "walu", "number"}
	for _, m := range modes {
		t.Run(m, func(t *testing.T) {
			// モードに応じた初期化ロジックなどが分離されていればここでテスト可能
		})
	}
}

func TestShort(t *testing.T) {
	if testing.Short() {
		t.Log("Skipping long running E2E tests in short mode")
		return
	}
	// ここにDB作成などを伴う重いテストを書く
}

func TestGetAlphaNumCount(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"abc123ABC", 9},
		{"a!b@c#", 3},
		{"", 0},
		{"123 456", 6},
	}
	for _, tt := range tests {
		got := getAlphaNumCount(tt.input)
		if got != tt.want {
			t.Errorf("getAlphaNumCount(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestGetNonAlnumCount(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"abc123ABC", 0},
		{"a!b@c#", 3},
		{" ", 1},
		{"", 0},
	}
	for _, tt := range tests {
		got := getNonAlnumCount(tt.input)
		if got != tt.want {
			t.Errorf("getNonAlnumCount(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestGetMaxNonAlnumLength(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"abc!!!def", 3},
		{"!!!abc!!!", 3},
		{"abc", 0},
		{"!!!", 3},
		{"", 0},
	}
	for _, tt := range tests {
		got := getMaxNonAlnumLength(tt.input)
		if got != tt.want {
			t.Errorf("getMaxNonAlnumLength(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestGetKeywordsVector(t *testing.T) {
	s := "select * from users where id = 1"
	keys := []string{"select", "from", "where", "insert"}
	want := []float64{1, 1, 1, 0}
	got := getKeywordsVector(&s, &keys)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("getKeywordsVector() = %v, want %v", got, want)
	}
}

func TestGetWaluVector(t *testing.T) {
	// Walu vector depends on getCharCount, etc.
	// We just check if it returns a vector of expected length for a typical log line.
	s := `127.0.0.1 - - [04/May/2026:10:30:00 +0900] "GET /path?query=1 HTTP/1.1" 200 123`
	got := getWaluVector(&s)
	if len(got) == 0 {
		t.Errorf("getWaluVector() returned empty vector for valid input")
	}
	// The length should be 29
	if len(got) != 29 {
		t.Errorf("getWaluVector() length = %d, want 29", len(got))
	}
}
