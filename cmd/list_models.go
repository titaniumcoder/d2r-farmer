package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var listModelsWithProvider = listModelsForProvider

var listModelsCmd = &cobra.Command{
	Use:   "list-models [provider]",
	Short: "List available models for a provider",
	Long:  "List the models available from the configured provider. Currently only OpenAI is supported.",
	Args:  cobra.ExactArgs(1),
	RunE:  runListModels,
}

func init() {
	rootCmd.AddCommand(listModelsCmd)
}

func runListModels(cmd *cobra.Command, args []string) error {
	provider := strings.ToLower(strings.TrimSpace(args[0]))
	if provider == "" {
		return fmt.Errorf("provider cannot be empty")
	}

	return listModelsWithProvider(cmd, provider)
}

func listModelsForProvider(cmd *cobra.Command, provider string) error {
	switch provider {
	case "openai":
		return listOpenAIModels(cmd)
	default:
		return fmt.Errorf("unsupported provider %q (only openai is supported for now)", provider)
	}
}

func listOpenAIModels(cmd *cobra.Command) error {
	cfg, err := readConfig()
	if err != nil {
		return err
	}

	client := newOpenAIClient(cfg.OpenAI)
	pager := client.Models.ListAutoPaging(cmd.Context())

	count := 0
	for pager.Next() {
		count++
		cmd.Println(pager.Current().ID)
	}
	if err := pager.Err(); err != nil {
		return fmt.Errorf("list openai models: %w", err)
	}
	if count == 0 {
		cmd.Println("no models found")
	}
	return nil
}
