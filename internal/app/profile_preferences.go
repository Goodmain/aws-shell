package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	preferencesDirectoryName = "aws-shell"
	preferencesFileName      = "preferences.json"
)

type FileProfilePreferenceStore struct {
	userConfigDir func() (string, error)
	readFile      func(string) ([]byte, error)
	writeFile     func(string, []byte, os.FileMode) error
	mkdirAll      func(string, os.FileMode) error
}

type profilePreferences struct {
	LastUsedProfile string `json:"last_used_profile"`
}

func NewFileProfilePreferenceStore() *FileProfilePreferenceStore {
	return &FileProfilePreferenceStore{
		userConfigDir: os.UserConfigDir,
		readFile:      os.ReadFile,
		writeFile:     os.WriteFile,
		mkdirAll:      os.MkdirAll,
	}
}

func (s *FileProfilePreferenceStore) LoadLastUsedProfile() (string, error) {
	path, err := s.preferenceFilePath()
	if err != nil {
		return "", err
	}

	contents, err := s.readFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}

		return "", err
	}

	var preferences profilePreferences
	if err := json.Unmarshal(contents, &preferences); err != nil {
		return "", err
	}

	return strings.TrimSpace(preferences.LastUsedProfile), nil
}

func (s *FileProfilePreferenceStore) SaveLastUsedProfile(profile string) error {
	path, err := s.preferenceFilePath()
	if err != nil {
		return err
	}

	if err := s.mkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	payload, err := json.Marshal(profilePreferences{LastUsedProfile: strings.TrimSpace(profile)})
	if err != nil {
		return err
	}

	return s.writeFile(path, payload, 0o600)
}

func (s *FileProfilePreferenceStore) preferenceFilePath() (string, error) {
	configDir, err := s.userConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}

	return profilePreferencesPath(configDir), nil
}

func profilePreferencesPath(configDir string) string {
	return filepath.Join(configDir, preferencesDirectoryName, preferencesFileName)
}

type noopProfilePreferenceStore struct{}

func (noopProfilePreferenceStore) LoadLastUsedProfile() (string, error) {
	return "", nil
}

func (noopProfilePreferenceStore) SaveLastUsedProfile(string) error {
	return nil
}
