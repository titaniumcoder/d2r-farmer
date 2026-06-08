package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	initProvider string
	initAPIKey   string
	initModel    string
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize API provider setup",
	Long:  "Configure the LLM provider credentials used for gear enrichment.",
	Args:  cobra.NoArgs,
	RunE:  initProviderConfig,
}

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().StringVar(&initProvider, "provider", "openai", "LLM provider")
	initCmd.Flags().StringVar(&initAPIKey, "api-key", "", "Provider API key (or set OPENAI_API_KEY)")
	initCmd.Flags().StringVar(&initModel, "model", "gpt-4.1-mini", "OpenAI model used for gear enrichment")
}

func initProviderConfig(cmd *cobra.Command, args []string) error {
	provider := strings.ToLower(strings.TrimSpace(initProvider))
	if provider == "" {
		return fmt.Errorf("provider cannot be empty")
	}

	if provider != "openai" {
		return fmt.Errorf("unsupported provider %q (only openai is supported for now)", provider)
	}

	apiKey := strings.TrimSpace(initAPIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	if apiKey == "" {
		return fmt.Errorf("api key is required via --api-key or OPENAI_API_KEY")
	}

	model := strings.TrimSpace(initModel)
	if model == "" {
		model = "gpt-4.1-mini"
	}

	cfg := Config{
		Provider: provider,
		OpenAI: OpenAIConfig{
			APIKey: apiKey,
			Model:  model,
		},
	}

	if err := writeConfig(cfg); err != nil {
		return err
	}

	cmd.Printf("initialized provider %q in %s\n", provider, configPath())
	return nil
}
