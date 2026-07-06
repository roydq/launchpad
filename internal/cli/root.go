package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/launchpad/launchpad/pkg/apiclient"
	"github.com/spf13/cobra"
)

type Config struct {
	APIURL  string
	Token   string
	Project string
}

type localConfig struct {
	Project string `json:"project"`
}

func NewRoot(cfg Config) *cobra.Command {
	client := apiclient.New(cfg.APIURL, cfg.Token)

	root := &cobra.Command{
		Use:   "launchpad",
		Short: "Manage projects on Launchpad",
	}

	projectsCmd := &cobra.Command{Use: "projects", Short: "Manage projects"}
	projectsCmd.AddCommand(&cobra.Command{
		Use:   "create [name]",
		Short: "Create a new project (bootstraps dev env + primary service)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetType, _ := cmd.Flags().GetString("target")
			namespace, _ := cmd.Flags().GetString("namespace")
			project, err := client.CreateProject(cmd.Context(), args[0], targetType, namespace)
			if err != nil {
				return err
			}
			fmt.Printf("created project %s (%s)\n", project.Name, project.ID)
			return nil
		},
	})
	createFlags := projectsCmd.Commands()[0].Flags()
	createFlags.String("target", "stub", "dev environment target type")
	createFlags.String("namespace", "default", "dev environment namespace")
	root.AddCommand(projectsCmd)

	root.AddCommand(&cobra.Command{
		Use:   "use [project]",
		Short: "Set the active project in ~/.launchpad/config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := saveLocalConfig(localConfig{Project: args[0]}); err != nil {
				return err
			}
			fmt.Printf("using project %s\n", args[0])
			return nil
		},
	})

	configCmd := &cobra.Command{Use: "config", Short: "Manage service config in dev environment"}
	configCmd.AddCommand(&cobra.Command{
		Use:   "get",
		Short: "Show config vars",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			vars, err := client.GetConfig(cmd.Context(), project)
			if err != nil {
				return err
			}
			b, _ := json.MarshalIndent(vars, "", "  ")
			fmt.Println(string(b))
			return nil
		},
	})
	configCmd.AddCommand(&cobra.Command{
		Use:   "set [KEY=VALUE...]",
		Short: "Set config vars",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			updates := make(map[string]*string, len(args))
			for _, arg := range args {
				parts := strings.SplitN(arg, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("expected KEY=VALUE, got %q", arg)
				}
				val := parts[1]
				updates[parts[0]] = &val
			}
			vars, err := client.PatchConfig(cmd.Context(), project, updates)
			if err != nil {
				return err
			}
			b, _ := json.MarshalIndent(vars, "", "  ")
			fmt.Println(string(b))
			return nil
		},
	})
	root.AddCommand(configCmd)

	deployCmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy an image immediately",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			image, _ := cmd.Flags().GetString("image")
			result, err := client.Deploy(cmd.Context(), project, image, "cli deploy")
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

	root.AddCommand(&cobra.Command{
		Use:   "ps",
		Short: "List processes for the active project",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			processes, err := client.ListProcesses(cmd.Context(), project)
			if err != nil {
				return err
			}
			b, _ := json.MarshalIndent(processes, "", "  ")
			fmt.Println(string(b))
			return nil
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "releases",
		Short: "List releases for the active project",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			releases, err := client.ListReleases(cmd.Context(), project)
			if err != nil {
				return err
			}
			b, _ := json.MarshalIndent(releases, "", "  ")
			fmt.Println(string(b))
			return nil
		},
	})

	changesetCmd := &cobra.Command{Use: "changeset", Short: "Stage changes before deploying (git-like workflow)"}

	changesetCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show staged changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			cs, err := client.GetChangeset(cmd.Context(), project)
			if err != nil {
				return err
			}
			b, _ := json.MarshalIndent(cs, "", "  ")
			fmt.Println(string(b))
			return nil
		},
	})

	addCmd := &cobra.Command{
		Use:   "add [KEY=VALUE...]",
		Short: "Stage config, scale, or image changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			changes, err := parseStageArgs(args, cmd)
			if err != nil {
				return err
			}
			cs, err := client.StageChanges(cmd.Context(), project, changes)
			if err != nil {
				return err
			}
			n := 0
			if ch, ok := cs["Changes"].([]any); ok {
				n = len(ch)
			} else if ch, ok := cs["changes"].([]any); ok {
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
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			if err := client.DiscardChangeset(cmd.Context(), project); err != nil {
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
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			desc, _ := cmd.Flags().GetString("message")
			result, err := client.PushChangeset(cmd.Context(), project, desc)
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

func requireProject(cfg Config) (string, error) {
	if cfg.Project == "" {
		return "", fmt.Errorf("set project with `launchpad use <name>` or LAUNCHPAD_PROJECT")
	}
	return cfg.Project, nil
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

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".launchpad", "config"), nil
}

func loadLocalConfig() (localConfig, error) {
	path, err := configPath()
	if err != nil {
		return localConfig{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return localConfig{}, nil
		}
		return localConfig{}, err
	}
	var cfg localConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return localConfig{}, err
	}
	return cfg, nil
}

func saveLocalConfig(cfg localConfig) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func LoadConfig() Config {
	cfg := Config{
		APIURL: envOr("LAUNCHPAD_API_URL", "http://localhost:8080"),
		Token:  os.Getenv("LAUNCHPAD_TOKEN"),
	}
	if local, err := loadLocalConfig(); err == nil {
		cfg.Project = local.Project
	}
	if v := os.Getenv("LAUNCHPAD_PROJECT"); v != "" {
		cfg.Project = v
	}
	return cfg
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func MustRun(cfg Config) {
	if err := NewRoot(cfg).Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}