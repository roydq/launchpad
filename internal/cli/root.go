package cli

import (
	"encoding/json"
	"errors"
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

	newCmd := &cobra.Command{
		Use:   "new [recipe|list] [project]",
		Short: "Create a project from a recipe template",
		Long: `Bootstrap a project from a built-in recipe (CLI-local templates).

  launchpad new list
  launchpad new hello-stub my-api
  launchpad new my-api              # default recipe: hello-stub
  launchpad new web-stub my-web     # stages PORT=8080 + image`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			parsed, err := ParseNewArgs(args)
			if err != nil {
				return err
			}
			if parsed.List {
				printRecipeList()
				return nil
			}
			recipe, ok := LookupRecipe(parsed.Recipe)
			if !ok {
				return fmt.Errorf("unknown recipe %q; run \"launchpad new list\"", parsed.Recipe)
			}
			targetType, _ := cmd.Flags().GetString("target")
			namespace, _ := cmd.Flags().GetString("namespace")
			noStage, _ := cmd.Flags().GetBool("no-stage")
			dir, _ := cmd.Flags().GetString("dir")
			// Ensure staging uses the project's default env (dev).
			prevEnv := client.Environment
			client.Environment = "dev"
			defer func() { client.Environment = prevEnv }()
			return ApplyRecipe(cmd.Context(), client, recipe, parsed.Project, ApplyRecipeOptions{
				TargetType: targetType,
				Namespace:  namespace,
				NoStage:    noStage,
				Dir:        dir,
			})
		},
	}
	// Empty default means "use recipe defaults" for target/namespace.
	newCmd.Flags().String("target", "", "override recipe target type (default: recipe target)")
	newCmd.Flags().String("namespace", "", "override recipe namespace (default: recipe namespace)")
	newCmd.Flags().Bool("no-stage", false, "create project only; do not stage image/config")
	newCmd.Flags().String("dir", ".", "write project-local .launchpad/config here (empty to skip)")
	root.AddCommand(newCmd)

	root.AddCommand(&cobra.Command{
		Use:   "use [project]",
		Short: "Set the active project in ~/.launchpad/config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env := effectiveEnv(cfg)
			if err := saveActiveContext(args[0], env); err != nil {
				return err
			}
			fmt.Printf("using project %s (environment %s)\n", args[0], env)
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

	envClone := &cobra.Command{
		Use:   "clone [from] [to]",
		Short: "Clone an environment (plain config; secrets listed as needs_value)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			targetType, _ := cmd.Flags().GetString("target")
			namespace, _ := cmd.Flags().GetString("namespace")
			ephemeral, _ := cmd.Flags().GetBool("ephemeral")
			result, err := client.CloneEnvironment(cmd.Context(), project, args[0], args[1], targetType, namespace, ephemeral)
			if err != nil {
				return err
			}
			fmt.Printf("created environment %s from %s (target=%s)\n", result.Environment.Name, result.From, result.Environment.TargetType)
			if len(result.ClonedPlain) > 0 {
				fmt.Printf("cloned plain: %s\n", strings.Join(result.ClonedPlain, ", "))
			} else {
				fmt.Println("cloned plain: (none)")
			}
			if len(result.NeedsValue) > 0 {
				fmt.Printf("needs_value (secrets): %s\n", strings.Join(result.NeedsValue, ", "))
				fmt.Printf("next: launchpad env use %s && launchpad config set --secret KEY=...\n", result.Environment.Name)
			} else {
				fmt.Printf("next: launchpad env use %s && launchpad deploy --image <ref>\n", result.Environment.Name)
			}
			return nil
		},
	}
	envClone.Flags().String("target", "", "override target type (default: copy from source)")
	envClone.Flags().String("namespace", "", "override namespace (default: copy from source)")
	envClone.Flags().Bool("ephemeral", false, "mark destination as ephemeral")
	envCmd.AddCommand(envClone)

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
			if err := saveActiveContext(project, name); err != nil {
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
	configGetCmd := &cobra.Command{
		Use:   "get",
		Short: "Show config vars (resolved by default; secrets as [secret])",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			layer := ""
			if shared, _ := cmd.Flags().GetBool("shared"); shared {
				layer = "shared"
			}
			if serviceOnly, _ := cmd.Flags().GetBool("service"); serviceOnly {
				layer = "service"
			}
			vars, err := client.GetConfigLayer(cmd.Context(), project, layer)
			if err != nil {
				return err
			}
			// Present API sentinel *** as [secret] for humans.
			display := make(map[string]string, len(vars))
			for k, v := range vars {
				if v == "***" {
					display[k] = "[secret]"
				} else {
					display[k] = v
				}
			}
			b, _ := json.MarshalIndent(display, "", "  ")
			fmt.Println(string(b))
			return nil
		},
	}
	configGetCmd.Flags().Bool("shared", false, "show shared layer only")
	configGetCmd.Flags().Bool("service", false, "show service layer only")
	configCmd.AddCommand(configGetCmd)

	configSetCmd := &cobra.Command{
		Use:   "set [KEY=VALUE...]",
		Short: "Stage config vars (use --now to release immediately)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			layer := "service"
			if shared, _ := cmd.Flags().GetBool("shared"); shared {
				layer = "shared"
			}
			asSecret, _ := cmd.Flags().GetBool("secret")
			asPlain, _ := cmd.Flags().GetBool("plain")
			if asSecret && asPlain {
				return fmt.Errorf("use only one of --secret or --plain")
			}
			sensitivity := ""
			if asSecret {
				sensitivity = "secret"
			} else if asPlain {
				sensitivity = "plain"
			}
			changes, err := parseKEYVALArgsLayer(args, layer, sensitivity)
			if err != nil {
				return err
			}
			now, _ := cmd.Flags().GetBool("now")
			message, _ := cmd.Flags().GetString("message")
			wait, timeout := waitFlags(cmd)
			label := "config"
			if layer == "shared" {
				label = "shared config"
			}
			if asSecret {
				label += " (secret)"
			}
			return stageAndMaybeNow(cmd.Context(), client, project, changes, now, message,
				fmt.Sprintf("Staged %s %s", label, configKeysSummary(changes)), wait, timeout)
		},
	}
	configSetCmd.Flags().Bool("shared", false, "stage into shared (project×env) layer")
	configSetCmd.Flags().Bool("secret", false, "mark keys as secret (redacted on read)")
	configSetCmd.Flags().Bool("plain", false, "mark keys as plain (explicit demote from secret)")
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

	diffCmd := &cobra.Command{
		Use:   "diff",
		Short: "Show pending staged changes vs last deploy (or compare releases / envs)",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			fromV, _ := cmd.Flags().GetInt("from-release")
			toV, _ := cmd.Flags().GetInt("to-release")
			fromEnv, _ := cmd.Flags().GetString("from-env")
			toEnv, _ := cmd.Flags().GetString("to-env")
			hasRelease := fromV > 0 || toV > 0
			hasEnv := fromEnv != "" || toEnv != ""
			if hasRelease && hasEnv {
				return fmt.Errorf("cannot combine --from-release/--to-release with --from-env/--to-env")
			}
			if hasEnv {
				if fromEnv == "" || toEnv == "" {
					return fmt.Errorf("both --from-env and --to-env are required for environment compare")
				}
				prev, err := client.PreviewEnvironments(cmd.Context(), project, fromEnv, toEnv)
				if err != nil {
					return err
				}
				fromLabel := fromEnv
				toLabel := toEnv
				if prev.FromVersion != nil {
					fromLabel = fmt.Sprintf("%s (v%d)", fromEnv, *prev.FromVersion)
				} else {
					fromLabel = fromEnv + " (none)"
				}
				if prev.ToVersion != nil {
					toLabel = fmt.Sprintf("%s (v%d)", toEnv, *prev.ToVersion)
				} else {
					toLabel = toEnv + " (none)"
				}
				fmt.Printf("# env %s → %s\n", fromLabel, toLabel)
				fmt.Print(prev.Summary)
				return nil
			}
			if hasRelease {
				if fromV < 1 || toV < 1 {
					return fmt.Errorf("both --from-release and --to-release are required for release compare")
				}
				prev, err := client.PreviewReleases(cmd.Context(), project, fromV, toV)
				if err != nil {
					return err
				}
				fmt.Printf("# release v%d → v%d\n", fromV, toV)
				fmt.Print(prev.Summary)
				return nil
			}
			// Server-side fold/diff (agents use the same API).
			prev, err := client.PreviewPending(cmd.Context(), project)
			if err != nil {
				return err
			}
			fmt.Print(prev.Summary)
			return nil
		},
	}
	diffCmd.Flags().Int("from-release", 0, "compare release versions (baseline)")
	diffCmd.Flags().Int("to-release", 0, "compare release versions (target)")
	diffCmd.Flags().String("from-env", "", "compare last deploy in environment (baseline)")
	diffCmd.Flags().String("to-env", "", "compare last deploy in environment (target)")
	root.AddCommand(diffCmd)

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
			if err := confirmSensitiveEnv(effectiveEnv(cfg), yesFlag(cmd)); err != nil {
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
	addYesFlag(deployCmd)
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
		Use:   "unstage",
		Short: "Remove the most recently staged change (keep the rest of the batch)",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			res, err := client.UnstageLastChange(cmd.Context(), project)
			if err != nil {
				if strings.Contains(err.Error(), "status 404") || strings.Contains(err.Error(), "not found") {
					fmt.Println("nothing to unstage")
					return nil
				}
				return err
			}
			desc := formatUnstagedChange(res)
			if res.RemainingCount == 0 {
				fmt.Printf("unstaged %s (staging empty)\n", desc)
			} else {
				fmt.Printf("unstaged %s (%d pending remaining)\n", desc, res.RemainingCount)
			}
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
		Use:   "logs [process]",
		Short: "Show process logs for the current environment (default web)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			process := "web"
			if len(args) == 1 {
				process = args[0]
			}
			body, err := client.GetLogs(cmd.Context(), project, process)
			if err != nil {
				return err
			}
			fmt.Print(body)
			return nil
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "inspect",
		Short: "Show project@env snapshot: pending, last deploy, processes",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInspect(cmd.Context(), client, cfg)
		},
	})

	releasesCmd := &cobra.Command{
		Use:   "releases",
		Short: "List or show releases for the active project",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listReleasesJSON(cmd.Context(), client, cfg)
		},
	}
	releasesCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List releases (JSON)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listReleasesJSON(cmd.Context(), client, cfg)
		},
	})
	releasesCmd.AddCommand(&cobra.Command{
		Use:   "show [version]",
		Short: "Show full release snapshot for a version",
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
			rel, err := findReleaseByVersion(cmd.Context(), client, project, version)
			if err != nil {
				return err
			}
			b, _ := json.MarshalIndent(rel, "", "  ")
			fmt.Println(string(b))
			return nil
		},
	})
	root.AddCommand(releasesCmd)

	rollbackCmd := &cobra.Command{
		Use:   "rollback [version]",
		Short: "Create a new release from a prior version and deploy to current env",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			if err := confirmSensitiveEnv(effectiveEnv(cfg), yesFlag(cmd)); err != nil {
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
	addYesFlag(rollbackCmd)
	root.AddCommand(rollbackCmd)

	promoteCmd := &cobra.Command{
		Use:   "promote",
		Short: "Promote a succeeded release from one env to another (re-resolves target config)",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, err := requireProject(cfg)
			if err != nil {
				return err
			}
			from, _ := cmd.Flags().GetString("from")
			if from == "" {
				return fmt.Errorf("--from is required")
			}
			to, _ := cmd.Flags().GetString("to")
			if to == "" {
				to = effectiveEnv(cfg)
			}
			if err := confirmSensitiveEnv(to, yesFlag(cmd)); err != nil {
				return err
			}
			version, _ := cmd.Flags().GetInt("release")
			message, _ := cmd.Flags().GetString("message")
			result, err := client.Promote(cmd.Context(), project, from, to, version, message)
			if err != nil {
				return err
			}
			wait, timeout := waitFlags(cmd)
			return maybeWaitForDeploy(cmd.Context(), client, result, wait, timeout)
		},
	}
	promoteCmd.Flags().String("from", "", "source environment (required)")
	promoteCmd.Flags().String("to", "", "target environment (default: current env context)")
	promoteCmd.Flags().Int("release", 0, "source release version (default: running in --from)")
	promoteCmd.Flags().StringP("message", "m", "", "release description")
	_ = promoteCmd.MarkFlagRequired("from")
	addWaitFlags(promoteCmd)
	addYesFlag(promoteCmd)
	root.AddCommand(promoteCmd)

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

	promptCmd := &cobra.Command{
		Use:   "prompt",
		Short: "Print project@env for shell prompts (no network)",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			out := FormatPrompt(cfg, format)
			if out != "" {
				fmt.Println(out)
			}
			return nil
		},
	}
	promptCmd.Flags().String("format", "short", "output format: short (project@env) or long")
	root.AddCommand(promptCmd)

	root.AddCommand(&cobra.Command{
		Use:   "shell-init [bash|zsh]",
		Short: "Print shell snippet to show (lp:project@env) in PS1",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			shell := "bash"
			if len(args) == 1 {
				shell = args[0]
			}
			script, err := ShellInitScript(shell)
			if err != nil {
				return err
			}
			fmt.Print(script)
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

// saveActiveContext writes project/env to global config and, when present,
// updates any walk-up project-local .launchpad/config so env use is not
// shadowed by a stale local environment field (e.g. after launchpad new).
func saveActiveContext(project, environment string) error {
	local, _ := loadLocalConfig()
	local.Project = project
	if environment != "" {
		local.Environment = environment
	}
	if err := saveLocalConfig(local); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	pl, path, err := findProjectLocalConfig(cwd)
	if err != nil || path == "" {
		return nil
	}
	pl.Project = project
	if environment != "" {
		pl.Environment = environment
	}
	data, err := json.MarshalIndent(pl, "", "  ")
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
		fmt.Fprint(os.Stderr, formatCLIError(err))
		os.Exit(1)
	}
}

// formatCLIError renders API problem+json recovery hints for humans and agents.
func formatCLIError(err error) string {
	var ae *apiclient.APIError
	if errors.As(err, &ae) && ae != nil {
		var b strings.Builder
		b.WriteString("error: ")
		b.WriteString(ae.Error())
		b.WriteByte('\n')
		if len(ae.Hints) > 0 {
			b.WriteString("recovery:\n")
			for _, h := range ae.Hints {
				b.WriteString("  - ")
				if h.Message != "" {
					b.WriteString(h.Message)
				} else {
					b.WriteString(h.Action)
				}
				if h.Command != "" {
					b.WriteString("\n    try: ")
					b.WriteString(h.Command)
				}
				b.WriteByte('\n')
			}
		}
		return b.String()
	}
	return fmt.Sprintf("error: %v\n", err)
}

// sensitiveEnvironments require explicit --yes for mutating CLI deploy paths.
var sensitiveEnvironments = map[string]struct{}{
	"production": {},
	"prod":       {},
}

func isSensitiveEnv(name string) bool {
	_, ok := sensitiveEnvironments[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

func addYesFlag(cmd *cobra.Command) {
	cmd.Flags().Bool("yes", false, "confirm mutations to sensitive environments (production)")
}

func yesFlag(cmd *cobra.Command) bool {
	yes, _ := cmd.Flags().GetBool("yes")
	return yes
}

func confirmSensitiveEnv(env string, yes bool) error {
	if !isSensitiveEnv(env) {
		return nil
	}
	if yes {
		return nil
	}
	return fmt.Errorf("refusing to modify sensitive environment %q without --yes (set LAUNCHPAD_ENV or --to to a non-production env, or pass --yes)", env)
}
