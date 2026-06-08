package cmd

import "testing"

func TestRootCommandHasCoreSubcommands(t *testing.T) {
	if got := rootCmd.Use; got != "d2r-farmer" {
		t.Fatalf("expected root command use to be d2r-farmer, got %q", got)
	}

	if got := rootCmd.Commands(); len(got) == 0 {
		t.Fatalf("expected at least one subcommand, got %d", len(got))
	}

	if got := rootCmd.CommandPath(); got != "d2r-farmer" {
		t.Fatalf("expected root command path to be d2r-farmer, got %q", got)
	}

	if cmd, _, err := rootCmd.Find([]string{"init"}); err != nil || cmd == nil {
		t.Fatalf("expected init command to exist, got err=%v", err)
	}

	if cmd, _, err := rootCmd.Find([]string{"char"}); err != nil || cmd == nil {
		t.Fatalf("expected char command to exist, got err=%v", err)
	}

	if cmd, _, err := rootCmd.Find([]string{"gear"}); err != nil || cmd == nil {
		t.Fatalf("expected gear command to exist, got err=%v", err)
	}

	if cmd, _, err := rootCmd.Find([]string{"list"}); err != nil || cmd == nil {
		t.Fatalf("expected list command to exist, got err=%v", err)
	}

	if cmd, _, err := rootCmd.Find([]string{"list-models"}); err != nil || cmd == nil {
		t.Fatalf("expected list-models command to exist, got err=%v", err)
	}
}
