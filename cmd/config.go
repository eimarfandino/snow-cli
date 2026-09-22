package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	survey "github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
)

type Config struct {
	SNInstance      string `json:"sn_instance"`
	StdTemplateID   string `json:"std_template_id"`
	CmdbCI          string `json:"cmdb_ci"`
	CmdbCISysID     string `json:"cmdb_ci_sys_id"`
	AssignmentGroup string `json:"assignment_group"`
	AssignedTo      string `json:"assigned_to"`
}

var appConfig Config

func configPath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".mkcr")
	os.MkdirAll(dir, 0700)
	return filepath.Join(dir, "config.json")
}

func configExists() bool {
	_, err := os.Stat(configPath())
	return err == nil
}

func loadConfig() error {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &appConfig)
}

func saveConfig(c Config) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0600)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Set up mkcr — required before login or create",
	Run: func(cmd *cobra.Command, args []string) {
		defaults := Config{}

		// Load existing config as defaults if already set up, so re-running config edits instead of resetting
		if configExists() {
			if err := loadConfig(); err == nil {
				defaults = appConfig
			}
		}

		var answers Config

		must(survey.AskOne(&survey.Input{
			Message: "ServiceNow instance:",
			Default: defaults.SNInstance,
		}, &answers.SNInstance))

		must(survey.AskOne(&survey.Input{
			Message: "Standard Change template sys_id:",
			Default: defaults.StdTemplateID,
		}, &answers.StdTemplateID))

		must(survey.AskOne(&survey.Input{
			Message: "Configuration item (cmdb_ci display name):",
			Default: defaults.CmdbCI,
		}, &answers.CmdbCI))

		must(survey.AskOne(&survey.Input{
			Message: "Configuration item sys_id (find it in the URL when opening the CI in ServiceNow):",
			Default: defaults.CmdbCISysID,
		}, &answers.CmdbCISysID))

		must(survey.AskOne(&survey.Input{
			Message: "Assignment group:",
			Default: defaults.AssignmentGroup,
		}, &answers.AssignmentGroup))

		must(survey.AskOne(&survey.Input{
			Message: "Assigned to (full name as it appears in ServiceNow):",
			Default: defaults.AssignedTo,
		}, &answers.AssignedTo))

		must(saveConfig(answers))
		fmt.Println("Config saved to", configPath())
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
}
