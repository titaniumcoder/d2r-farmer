package cmd
package cmd

import "testing"

func TestRootCommandHasNoSubcommands(t *testing.T) {
	if got := len(rootCmd.Commands()); got != 0 {
		t.Fatalf("expected no subcommands, got %d", got)
	}

	if got := rootCmd.Use; got != "d2r-farmer" {
		t.Fatalf("expected root command use to be d2r-farmer, got %q", got)
	}
}