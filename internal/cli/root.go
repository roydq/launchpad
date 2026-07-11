package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/launchpad/launchpad/pkg/apiclient"
	"github.com/spf13/cobra"
)

type Config struct {
	APIURL      string
	Token       string
	Project     string
	Environment string
}

type localConfig struct {
	Project     string `json:"project"`
	Environment string `json:"environment,omitempty"`
}

func NewRoot(cfg Config) *cobra.Command {
	client := apiclient.New(cfg.APIURL, cfg.Token)
	client.Environment = cfg.Environment

	root := &cobra.Command{
		Use:   "launchpad",
		Short: "Manage projects on Launchpad",
	}

	projectsCmd := &cobra.Command{Use: "projects", Short: "Manage projects"}
	createCmd := &cobra.Command{
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
	}
	createCmd.Flags().String("target", "stub", "dev environment target type")
	createCmd.Flags().String("namespace", "default", "dev environment namespace")
	projectsCmd.AddCommand(createCmd)
	root.AddCommand(projectsCmd)

	root.AddCommand(&cobra.Command{
		Use:   "use [project]",
		Short: "Set the active project in ~/.launchpad/config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			local, _ := loadLocalConfig()
			local.Project = args[0]
			if err := saveLocalConfig(local); err != nil {
				return err
			}
			fmt.Printf("using project %s (environment %s)\n", args[0], effectiveEnv(cfg))
			return nil
		},
	})

	envCmd := &cobra.Command{Use: "env", Short: "Manage environments (ambient deploy/config context)"}
	envCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List environments for the active project",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			envs, err := client.ListEnvironments(cmd.Context(), project)
			if err != nil {
				return err
			}
			cur := effectiveEnv(cfg)
			for _, e := range envs {
				mark := " "
				if e.Name == cur {
					mark = "*"
				}
				fmt.Printf("%s %s  target=%s\n", mark, e.Name, e.TargetType)
			}
			return nil
		},
	})
	envCreate := &cobra.Command{
		Use:   "create [name]",
		Short: "Create an environment (empty config; own target)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			targetType, _ := cmd.Flags().GetString("target")
			namespace, _ := cmd.Flags().GetString("namespace")
			env, err := client.CreateEnvironment(cmd.Context(), project, args[0], targetType, namespace, false)
			if err != nil {
				return err
			}
			fmt.Printf("created environment %s (%s)\n", env.Name, env.ID)
			return nil
		},
	}
	envCreate.Flags().String("target", "stub", "target type")
	envCreate.Flags().String("namespace", "default", "target namespace")
	envCmd.AddCommand(envCreate)
	envCmd.AddCommand(&cobra.Command{
		Use:   "use [name]",
		Short: "Set the active environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			name := args[0]
			if _, err := client.GetEnvironment(cmd.Context(), project, name); err != nil {
				return err
			}
			cs, err := loadPending(cmd.Context(), client, project)
			if err != nil {
				return err
			}
			if pendingCount(cs) > 0 && cs.Environment != "" && cs.Environment != name {
				return fmt.Errorf("cannot switch environment: %d pending change(s) for %s; run \"launchpad deploy\", \"launchpad diff\", or \"launchpad reset\"",
					pendingCount(cs), cs.Environment)
			}
			local, _ := loadLocalConfig()
			local.Project = project
			local.Environment = name
			if err := saveLocalConfig(local); err != nil {
				return err
			}
			fmt.Printf("using environment %s\n", name)
			return nil
		},
	})
	envCmd.AddCommand(&cobra.Command{
		Use:   "current",
		Short: "Print the active environment",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(effectiveEnv(cfg))
			return nil
		},
	})
	root.AddCommand(envCmd)

	configCmd := &cobra.Command{Use: "config", Short: "Manage service config (stages by default)"}
	configCmd.AddCommand(&cobra.Command{
		Use:   "get",
		Short: "Show live (applied) config vars",
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

	configSetCmd := &cobra.Command{
		Use:   "set [KEY=VALUE...]",
		Short: "Stage config vars (use --now to release immediately)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			changes, err := parseKEYVALArgs(args)
			if err != nil {
				return err
			}
			now, _ := cmd.Flags().GetBool("now")
			message, _ := cmd.Flags().GetString("message")
			wait, timeout := waitFlags(cmd)
			return stageAndMaybeNow(cmd.Context(), client, project, changes, now, message,
				fmt.Sprintf("Staged config %s", configKeysSummary(changes)), wait, timeout)
		},
	}
	configSetCmd.Flags().Bool("now", false, "create a release immediately (requires clean staging)")
	configSetCmd.Flags().StringP("message", "m", "", "release description (with --now)")
	addWaitFlags(configSetCmd)
	configCmd.AddCommand(configSetCmd)

	configUnsetCmd := &cobra.Command{
		Use:   "unset [KEY...]",
		Short: "Stage config key deletions",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			changes := make([]map[string]any, 0, len(args))
			for _, key := range args {
				changes = append(changes, configUnsetChange(key))
			}
			now, _ := cmd.Flags().GetBool("now")
			message, _ := cmd.Flags().GetString("message")
			wait, timeout := waitFlags(cmd)
			return stageAndMaybeNow(cmd.Context(), client, project, changes, now, message,
				fmt.Sprintf("Staged unset %s", strings.Join(args, ", ")), wait, timeout)
		},
	}
	configUnsetCmd.Flags().Bool("now", false, "create a release immediately (requires clean staging)")
	configUnsetCmd.Flags().StringP("message", "m", "", "release description (with --now)")
	addWaitFlags(configUnsetCmd)
	configCmd.AddCommand(configUnsetCmd)
	root.AddCommand(configCmd)

	scaleCmd := &cobra.Command{
		Use:   "scale [PROC=N...]",
		Short: "Stage process scale changes",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			var changes []map[string]any
			var labels []string
			for _, arg := range args {
				ch, err := parseScaleArg(arg)
				if err != nil {
					return err
				}
				changes = append(changes, ch)
				labels = append(labels, arg)
			}
			now, _ := cmd.Flags().GetBool("now")
			message, _ := cmd.Flags().GetString("message")
			wait, timeout := waitFlags(cmd)
			return stageAndMaybeNow(cmd.Context(), client, project, changes, now, message,
				fmt.Sprintf("Staged scale %s", strings.Join(labels, ", ")), wait, timeout)
		},
	}
	scaleCmd.Flags().Bool("now", false, "create a release immediately (requires clean staging)")
	scaleCmd.Flags().StringP("message", "m", "", "release description (with --now)")
	addWaitFlags(scaleCmd)
	root.AddCommand(scaleCmd)

	imageCmd := &cobra.Command{
		Use:   "image [ref]",
		Short: "Stage a container image change",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			now, _ := cmd.Flags().GetBool("now")
			message, _ := cmd.Flags().GetString("message")
			wait, timeout := waitFlags(cmd)
			return stageAndMaybeNow(cmd.Context(), client, project, []map[string]any{imageChange(args[0])}, now, message,
				fmt.Sprintf("Staged image %s", args[0]), wait, timeout)
		},
	}
	imageCmd.Flags().Bool("now", false, "create a release immediately (requires clean staging)")
	imageCmd.Flags().StringP("message", "m", "", "release description (with --now)")
	addWaitFlags(imageCmd)
	root.AddCommand(imageCmd)

	root.AddCommand(&cobra.Command{
		Use:   "diff",
		Short: "Show pending staged changes vs last release",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			cs, err := loadPending(cmd.Context(), client, project)
			if err != nil {
				return err
			}
			folded, err := foldChanges(cs.Changes)
			if err != nil {
				return err
			}
			baseline, err := latestReleaseForEnv(cmd.Context(), client, project, effectiveEnv(cfg))
			if err != nil {
				return err
			}
			fmt.Print(formatDiff(folded, baseline))
			return nil
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show pending staged change summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			envName := effectiveEnv(cfg)
			fmt.Printf("project: %s\n", project)
			fmt.Printf("environment: %s\n", envName)
			cs, err := loadPending(cmd.Context(), client, project)
			if err != nil {
				return err
			}
			if baseline, err := latestReleaseForEnv(cmd.Context(), client, project, envName); err == nil && baseline != nil {
				fmt.Printf("running/last deploy: release v%d (%s)\n", baseline.Version, baseline.ArtifactRef)
			} else {
				fmt.Println("running/last deploy: none")
			}
			n := pendingCount(cs)
			if n == 0 {
				fmt.Println("No pending changes")
				return nil
			}
			pin := cs.Environment
			if pin == "" {
				pin = envName
			}
			var nConfig, nScale, nImage int
			for _, c := range cs.Changes {
				switch c.Type {
				case "config":
					nConfig++
				case "scale":
					nScale++
				case "image":
					nImage++
				}
			}
			fmt.Printf("pending: %d change(s) for %s (config=%d scale=%d image=%d)\n", n, pin, nConfig, nScale, nImage)
			fmt.Println(`Run "launchpad diff" to review, "launchpad deploy" to apply, "launchpad reset" to discard.`)
			return nil
		},
	})

	deployCmd := &cobra.Command{
		Use:   "deploy [KEY=VALUE...]",
		Short: "Submit staged changes as a release (optional one-shot mutations)",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			var changes []map[string]any
			if len(args) > 0 {
				kv, err := parseKEYVALArgs(args)
				if err != nil {
					return err
				}
				changes = append(changes, kv...)
			}
			image, _ := cmd.Flags().GetString("image")
			if image != "" {
				changes = append(changes, imageChange(image))
			}
			scale, _ := cmd.Flags().GetString("scale")
			if scale != "" {
				ch, err := parseScaleArg(scale)
				if err != nil {
					return fmt.Errorf("--scale: %w", err)
				}
				changes = append(changes, ch)
			}
			if len(changes) > 0 {
				if _, err := stage(cmd.Context(), client, project, changes); err != nil {
					return err
				}
			}
			cs, err := loadPending(cmd.Context(), client, project)
			if err != nil {
				return err
			}
			if pendingCount(cs) == 0 {
				return fmt.Errorf("nothing to deploy")
			}
			message, _ := cmd.Flags().GetString("message")
			result, err := push(cmd.Context(), client, project, message)
			if err != nil {
				return err
			}
			wait, timeout := waitFlags(cmd)
			return maybeWaitForDeploy(cmd.Context(), client, result, wait, timeout)
		},
	}
	deployCmd.Flags().String("image", "", "stage container image then deploy")
	deployCmd.Flags().String("scale", "", "stage scale change then deploy, e.g. web=3")
	deployCmd.Flags().StringP("message", "m", "", "release description")
	addWaitFlags(deployCmd)
	root.AddCommand(deployCmd)

	root.AddCommand(&cobra.Command{
		Use:   "reset",
		Short: "Discard all pending staged changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			if err := client.DiscardChangeset(cmd.Context(), project); err != nil {
				// Friendly no-op when nothing is open.
				if strings.Contains(err.Error(), "status 404") || strings.Contains(err.Error(), "not found") {
					fmt.Println("nothing to reset")
					return nil
				}
				return err
			}
			fmt.Println("pending changes discarded")
			return nil
		},
	})

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

	rollbackCmd := &cobra.Command{
		Use:   "rollback [version]",
		Short: "Create a new release from a prior version and deploy to current env",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			var version int
			if _, err := fmt.Sscanf(args[0], "%d", &version); err != nil || version < 1 {
				return fmt.Errorf("version must be a positive integer")
			}
			message, _ := cmd.Flags().GetString("message")
			result, err := client.Rollback(cmd.Context(), project, version, message)
			if err != nil {
				return err
			}
			wait, timeout := waitFlags(cmd)
			return maybeWaitForDeploy(cmd.Context(), client, result, wait, timeout)
		},
	}
	rollbackCmd.Flags().StringP("message", "m", "", "release description")
	addWaitFlags(rollbackCmd)
	root.AddCommand(rollbackCmd)

	root.AddCommand(&cobra.Command{
		Use:   "doctor",
		Short: "Check API connectivity, auth, and context",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd.Context(), client, cfg)
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "context",
		Short: "Show resolved project/environment and config sources",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("project: %s\n", emptyDash(cfg.Project))
			fmt.Printf("environment: %s\n", effectiveEnv(cfg))
			fmt.Printf("api: %s\n", cfg.APIURL)
			if cfg.Token != "" {
				fmt.Println("token: set")
			} else {
				fmt.Println("token: (not set)")
			}
			if cwd, err := os.Getwd(); err == nil {
				if _, path, err := findProjectLocalConfig(cwd); err == nil && path != "" {
					fmt.Printf("project-local: %s\n", path)
				} else {
					fmt.Println("project-local: (none)")
				}
			}
			if p, err := configPath(); err == nil {
				fmt.Printf("global-config: %s\n", p)
			}
			fmt.Println("precedence: LAUNCHPAD_* env > .launchpad/config (walk up) > ~/.launchpad/config")
			return nil
		},
	})

	return root
}

func emptyDash(s string) string {
	if s == "" {
		return "(not set)"
	}
	return s
}

func requireProject(cfg Config) (string, error) {
	if cfg.Project == "" {
		return "", fmt.Errorf("set project with `launchpad use <name>` or LAUNCHPAD_PROJECT")
	}
	return cfg.Project, nil
}

func effectiveEnv(cfg Config) string {
	if cfg.Environment != "" {
		return cfg.Environment
	}
	return "dev"
}

func addWaitFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("wait", false, "wait for deploy job to finish")
	cmd.Flags().Duration("timeout", 5*time.Minute, "max wait time with --wait")
}

func waitFlags(cmd *cobra.Command) (bool, time.Duration) {
	wait, _ := cmd.Flags().GetBool("wait")
	timeout, _ := cmd.Flags().GetDuration("timeout")
	return wait, timeout
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
	var global localConfig
	if g, err := loadLocalConfig(); err == nil {
		global = g
	}
	var projectLocal localConfig
	if cwd, err := os.Getwd(); err == nil {
		if pl, _, err := findProjectLocalConfig(cwd); err == nil {
			projectLocal = pl
		}
	}
	return mergeConfigLayers(global, projectLocal, os.Getenv("LAUNCHPAD_PROJECT"), os.Getenv("LAUNCHPAD_ENV"))
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
