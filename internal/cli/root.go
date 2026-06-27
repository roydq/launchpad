package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

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
		Short: "Deploy an application image immediately",
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

	scaleCmd := &cobra.Command{
		Use:   "scale [web=2]",
		Short: "Scale a process immediately",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.App == "" {
				return fmt.Errorf("set --app or LAUNCHPAD_APP")
			}
			parts := strings.SplitN(args[0], "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("expected process=quantity, got %q", args[0])
			}
			qty, err := strconv.Atoi(parts[1])
			if err != nil {
				return err
			}
			result, err := client.Scale(cmd.Context(), cfg.App, parts[0], qty)
			if err != nil {
				return err
			}
			fmt.Printf("scale queued: %v\n", result)
			return nil
		},
	}
	root.AddCommand(scaleCmd)

	rollbackCmd := &cobra.Command{
		Use:   "rollback [version]",
		Short: "Rollback to a previous release version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.App == "" {
				return fmt.Errorf("set --app or LAUNCHPAD_APP")
			}
			version, err := strconv.Atoi(args[0])
			if err != nil {
				return err
			}
			result, err := client.Rollback(cmd.Context(), cfg.App, version)
			if err != nil {
				return err
			}
			fmt.Printf("rollback queued: %v\n", result)
			return nil
		},
	}
	root.AddCommand(rollbackCmd)

	changesetCmd := &cobra.Command{Use: "changeset", Short: "Stage changes before deploying (git-like workflow)"}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show staged changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.App == "" {
				return fmt.Errorf("set --app or LAUNCHPAD_APP")
			}
			cs, err := client.GetChangeset(cmd.Context(), cfg.App)
			if err != nil {
				return err
			}
			b, _ := json.MarshalIndent(cs, "", "  ")
			fmt.Println(string(b))
			return nil
		},
	}
	changesetCmd.AddCommand(statusCmd)

	addCmd := &cobra.Command{
		Use:   "add [KEY=VALUE...]",
		Short: "Stage config, scale, or image changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.App == "" {
				return fmt.Errorf("set --app or LAUNCHPAD_APP")
			}
			changes, err := parseStageArgs(args, cmd)
			if err != nil {
				return err
			}
			cs, err := client.StageChanges(cmd.Context(), cfg.App, changes)
			if err != nil {
				return err
			}
			n := 0
			if ch, ok := cs["Changes"].([]any); ok {
				n = len(ch)
			}
			fmt.Printf("staged %d total change(s) in changeset\n", n)
			return nil
		},
	}
	addCmd.Flags().String("image", "", "stage a container image")
	addCmd.Flags().String("scale", "", "stage scale change, e.g. web=3")
	changesetCmd.AddCommand(addCmd)

	changesetCmd.AddCommand(&cobra.Command{
		Use:   "reset",
		Short: "Discard all staged changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.App == "" {
				return fmt.Errorf("set --app or LAUNCHPAD_APP")
			}
			if err := client.DiscardChangeset(cmd.Context(), cfg.App); err != nil {
				return err
			}
			fmt.Println("changeset discarded")
			return nil
		},
	})

	pushCmd := &cobra.Command{
		Use:   "push",
		Short: "Apply staged changes and deploy",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.App == "" {
				return fmt.Errorf("set --app or LAUNCHPAD_APP")
			}
			desc, _ := cmd.Flags().GetString("message")
			result, err := client.PushChangeset(cmd.Context(), cfg.App, desc)
			if err != nil {
				return err
			}
			fmt.Printf("push queued: %v\n", result)
			return nil
		},
	}
	pushCmd.Flags().String("message", "", "release description")
	changesetCmd.AddCommand(pushCmd)
	root.AddCommand(changesetCmd)

	return root
}

func parseStageArgs(args []string, cmd *cobra.Command) ([]map[string]any, error) {
	image, _ := cmd.Flags().GetString("image")
	scale, _ := cmd.Flags().GetString("scale")

	var changes []map[string]any
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("expected KEY=VALUE, got %q", arg)
		}
		changes = append(changes, map[string]any{"type": "config", "key": parts[0], "value": parts[1]})
	}
	if image != "" {
		changes = append(changes, map[string]any{"type": "image", "image": image})
	}
	if scale != "" {
		parts := strings.SplitN(scale, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("expected --scale web=3")
		}
		qty, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, err
		}
		changes = append(changes, map[string]any{"type": "scale", "process": parts[0], "quantity": qty})
	}
	if len(changes) == 0 {
		return nil, fmt.Errorf("no changes to stage")
	}
	return changes, nil
}

func MustRun(cfg Config) {
	if err := NewRoot(cfg).Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}