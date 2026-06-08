package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRunListModelsDispatchesProvider(t *testing.T) {
	prev := listModelsWithProvider
	t.Cleanup(func() {
		listModelsWithProvider = prev
	})

	var gotProvider string
	buf := &bytes.Buffer{}
	cmd := &cobra.Command{}
	cmd.SetOut(buf)

	listModelsWithProvider = func(cmd *cobra.Command, provider string) error {
		gotProvider = provider
		cmd.Println("gpt-4.1-mini")
		return nil
	}

	if err := runListModels(cmd, []string{"openai"}); err != nil {
		t.Fatalf("expected list-models to succeed, got: %v", err)
	}

	if gotProvider != "openai" {
		t.Fatalf("expected provider to be openai, got %q", gotProvider)
	}
	if !strings.Contains(buf.String(), "gpt-4.1-mini") {
		t.Fatalf("expected command output to contain model id, got: %s", buf.String())
	}
}

func TestRunListModelsRejectsUnsupportedProvider(t *testing.T) {
	if err := runListModels(&cobra.Command{}, []string{"anthropic"}); err == nil {
		t.Fatalf("expected unsupported provider to fail")
	}
}
