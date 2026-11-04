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
	LastUsedProfile  string            `json:"last_used_profile,omitempty"`
	LastUsedProvider string            `json:"last_used_provider,omitempty"`
	Profiles         map[string]string `json:"last_used_profiles,omitempty"`
}

func NewFileProfilePreferenceStore() *FileProfilePreferenceStore {
	return &FileProfilePreferenceStore{
		userConfigDir: os.UserConfigDir,
		readFile:      os.ReadFile,
		writeFile:     os.WriteFile,
		mkdirAll:      os.MkdirAll,
	}
}

func (s *FileProfilePreferenceStore) LoadLastUsedProfile(provider string) (string, error) {
	preferences, err := s.load()
	if err != nil {
		return "", err
	}
	if profile := strings.TrimSpace(preferences.Profiles[provider]); profile != "" {
		return profile, nil
	}
	if provider == AuthAWSVault {
		return strings.TrimSpace(preferences.LastUsedProfile), nil
	}
	return "", nil
}

func (s *FileProfilePreferenceStore) SaveLastUsedProfile(provider string, profile string) error {
	preferences, err := s.load()
	if err != nil {
		if !isInvalidPreferences(err) {
			return err
		}
		preferences = profilePreferences{}
	}
	if preferences.Profiles == nil {
		preferences.Profiles = make(map[string]string)
	}
	preferences.Profiles[provider] = strings.TrimSpace(profile)
	if provider == AuthAWSVault {
		preferences.LastUsedProfile = strings.TrimSpace(profile)
	}
	return s.save(preferences)
}

func (s *FileProfilePreferenceStore) LoadLastUsedProvider() (string, error) {
	preferences, err := s.load()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(preferences.LastUsedProvider), nil
}

func (s *FileProfilePreferenceStore) SaveLastUsedProvider(provider string) error {
	preferences, err := s.load()
	if err != nil {
		if !isInvalidPreferences(err) {
			return err
		}
		preferences = profilePreferences{}
	}
	preferences.LastUsedProvider = strings.TrimSpace(provider)
	return s.save(preferences)
}

func (s *FileProfilePreferenceStore) load() (profilePreferences, error) {
	path, err := s.preferenceFilePath()
	if err != nil {
		return profilePreferences{}, err
	}
	contents, err := s.readFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return profilePreferences{}, nil
		}
		return profilePreferences{}, err
	}
	var preferences profilePreferences
	if err := json.Unmarshal(contents, &preferences); err != nil {
		return profilePreferences{}, err
	}
	return preferences, nil
}

func isInvalidPreferences(err error) bool {
	var syntaxError *json.SyntaxError
	var typeError *json.UnmarshalTypeError
	return errors.As(err, &syntaxError) || errors.As(err, &typeError)
}

func (s *FileProfilePreferenceStore) save(preferences profilePreferences) error {
	path, err := s.preferenceFilePath()
	if err != nil {
		return err
	}
	if err := s.mkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	payload, err := json.Marshal(preferences)
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

func (noopProfilePreferenceStore) LoadLastUsedProfile(string) (string, error) {
	return "", nil
}

func (noopProfilePreferenceStore) SaveLastUsedProfile(string, string) error {
	return nil
}

func (noopProfilePreferenceStore) LoadLastUsedProvider() (string, error) {
	return "", nil
}

func (noopProfilePreferenceStore) SaveLastUsedProvider(string) error {
	return nil
}
