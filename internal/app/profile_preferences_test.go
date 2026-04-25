package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFileProfilePreferenceStoreLoadAndSave(t *testing.T) {
	tempDir := t.TempDir()
	store := NewFileProfilePreferenceStore()
	store.userConfigDir = func() (string, error) { return tempDir, nil }

	if err := store.SaveLastUsedProfile("dev"); err != nil {
		t.Fatalf("unexpected save error: %v", err)
	}

	profile, err := store.LoadLastUsedProfile()
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if profile != "dev" {
		t.Fatalf("got %q want %q", profile, "dev")
	}
}

func TestFileProfilePreferenceStoreLoadMissingFile(t *testing.T) {
	store := NewFileProfilePreferenceStore()
	store.userConfigDir = func() (string, error) { return t.TempDir(), nil }

	profile, err := store.LoadLastUsedProfile()
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if profile != "" {
		t.Fatalf("expected empty profile, got %q", profile)
	}
}

func TestFileProfilePreferenceStoreLoadInvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	store := NewFileProfilePreferenceStore()
	store.userConfigDir = func() (string, error) { return tempDir, nil }

	path := profilePreferencesPath(tempDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if _, err := store.LoadLastUsedProfile(); err == nil {
		t.Fatalf("expected json parse error")
	}
}

func TestProfilePreferencesPathSamples(t *testing.T) {
	tests := []struct {
		name      string
		configDir string
	}{
		{name: "linux-like", configDir: "/home/alex/.config"},
		{name: "macos-like", configDir: "/Users/alex/Library/Application Support"},
		{name: "windows-like", configDir: `C:\Users\alex\AppData\Roaming`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := profilePreferencesPath(tt.configDir)
			want := filepath.Join(tt.configDir, preferencesDirectoryName, preferencesFileName)
			if got != want {
				t.Fatalf("got %q want %q", got, want)
			}
		})
	}
}

func TestFileProfilePreferenceStoreConfigDirError(t *testing.T) {
	store := NewFileProfilePreferenceStore()
	store.userConfigDir = func() (string, error) { return "", errors.New("boom") }

	if _, err := store.LoadLastUsedProfile(); err == nil {
		t.Fatalf("expected config dir error")
	}
}
