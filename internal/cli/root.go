package cli

import (
	"fmt"
	"os"

	"github.com/launchpad/launchpad/pkg/apiclient"
	"github.com/spf13/cobra"
)

type Config struct {
	APIURL string
	Token  string
	Team   string
	App    string
}

func NewRoot(cfg Config) *cobra.Command {
	client := apiclient.New(cfg.APIURL, cfg.Token)

	root := &cobra.Command{
		Use:   "launchpad",
		Short: "Manage applications on Launchpad",
	}

	root.AddCommand(&cobra.Command{
		Use:   "apps:create [name]",
		Short: "Create a new application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := client.CreateApp(cmd.Context(), args[0], cfg.Team, "default")
			if err != nil {
				return err
			}
			fmt.Printf("created app %s (%s)\n", app.Name, app.ID)
			return nil
		},
	})

	deployCmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy an application image",
		RunE: func(cmd *cobra.Command, args []string) error {
			image, _ := cmd.Flags().GetString("image")
			if cfg.App == "" {
				return fmt.Errorf("set --app or LAUNCHPAD_APP")
			}
			result, err := client.Deploy(cmd.Context(), cfg.App, image, "cli deploy")
			if err != nil {
				return err
			}
			fmt.Printf("deployment queued: %v\n", result)
			return nil
		},
	}
	deployCmd.Flags().String("image", "", "container image to deploy")
	_ = deployCmd.MarkFlagRequired("image")
	root.AddCommand(deployCmd)

	return root
}

func MustRun(cfg Config) {
	if err := NewRoot(cfg).Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}