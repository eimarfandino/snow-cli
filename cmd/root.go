package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "mkcr",
	Short: "Create ServiceNow Standard Change requests without touching the UI",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Use == "config" {
			return nil // config command itself is exempt
		}
		if !configExists() {
			return fmt.Errorf("no config found — run: mkcr config")
		}
		return loadConfig()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
