package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitProviderConfigCreatesConfigFile(t *testing.T) {
	prevProvider := initProvider
	prevAPIKey := initAPIKey
	prevModel := initModel
	prevWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	temp := t.TempDir()
	if err := os.Chdir(temp); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(prevWd)
		initProvider = prevProvider
		initAPIKey = prevAPIKey
		initModel = prevModel
	})

	initProvider = "openai"
	initAPIKey = "test-key"
	initModel = "gpt-4.1-mini"

	if err := initProviderConfig(initCmd, nil); err != nil {
		t.Fatalf("expected init provider to succeed, got error: %v", err)
	}

	configFile := filepath.Join(temp, "data", "config.yaml")
	content, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("expected config file to exist at %s, got error: %v", configFile, err)
	}

	text := string(content)
	if !strings.Contains(text, "provider: openai") {
		t.Fatalf("expected provider field in file, got: %s", text)
	}
	if !strings.Contains(text, "api_key: test-key") {
		t.Fatalf("expected api key field in file, got: %s", text)
	}
	if !strings.Contains(text, "model: gpt-4.1-mini") {
		t.Fatalf("expected model field in file, got: %s", text)
	}
}

func TestInitProviderConfigRequiresAPIKey(t *testing.T) {
	prevProvider := initProvider
	prevAPIKey := initAPIKey
	prevWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	temp := t.TempDir()
	if err := os.Chdir(temp); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(prevWd)
		initProvider = prevProvider
		initAPIKey = prevAPIKey
		_ = os.Unsetenv("OPENAI_API_KEY")
	})

	initProvider = "openai"
	initAPIKey = ""
	_ = os.Unsetenv("OPENAI_API_KEY")

	err = initProviderConfig(initCmd, nil)
	if err == nil {
		t.Fatalf("expected missing api key to fail")
	}
	if !strings.Contains(err.Error(), "api key is required") {
		t.Fatalf("expected api key error, got: %v", err)
	}
}

func TestInitProviderConfigFromEnv(t *testing.T) {
	prevProvider := initProvider
	prevAPIKey := initAPIKey
	prevModel := initModel
	prevWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	temp := t.TempDir()
	if err := os.Chdir(temp); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(prevWd)
		initProvider = prevProvider
		initAPIKey = prevAPIKey
		initModel = prevModel
		_ = os.Unsetenv("OPENAI_API_KEY")
	})

	initProvider = "openai"
	initAPIKey = ""
	initModel = "gpt-4.1"
	_ = os.Setenv("OPENAI_API_KEY", "env-key")

	if err := initProviderConfig(initCmd, nil); err != nil {
		t.Fatalf("expected init provider to succeed with env key, got error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(temp, "data", "config.yaml"))
	if err != nil {
		t.Fatalf("expected config file to exist, got: %v", err)
	}
	if !strings.Contains(string(content), "api_key: env-key") {
		t.Fatalf("expected env api key to be stored, got: %s", string(content))
	}
}
