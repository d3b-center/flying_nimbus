/*
Copyright © 2026 D3B
*/
package cmd

import (
	"flying_nimbus/internal/app"
	"flying_nimbus/internal/tui"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	verbose     bool
	showVersion bool
	Version     string
	CommitHash  string
	Branch      string
	BuildDate   string
	Platform    string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "flying-nimbus",
	Short: "A developer-focused TUI for managing AWS infrastructure",
	Long: `Flying Nimbus is a terminal user interface (TUI) designed to streamline
day-to-day cloud infrastructure management with a strong emphasis on
developer workflows.`,
	Run: Run,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.flying_nimbus.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&showVersion, "version", false, "Enable verbose output")
}

func Run(cmd *cobra.Command, args []string) {
	if showVersion {
		PrintVersion(cmd, args)
		return
	}

	app, _ := app.InitApp(verbose)
	defer app.Shutdown()

	tui.StartTea(app)
}

func PrintVersion(cmd *cobra.Command, args []string) {
	fmt.Printf("Version: %s\nCommit Hash: %s\nBranch: %s\nBuild date: %s\nPlatform: %s\n",
		Version,
		CommitHash,
		Branch,
		BuildDate,
		Platform,
	)
}
