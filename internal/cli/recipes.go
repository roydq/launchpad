package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/launchpad/launchpad/pkg/apiclient"
)

// Recipe is a CLI-local project bootstrap template.
type Recipe struct {
	ID            string
	Description   string
	TargetType    string
	Namespace     string
	Image         string
	ServiceConfig map[string]string
}

var builtinRecipes = map[string]Recipe{
	"hello-stub": {
		ID:          "hello-stub",
		Description: "Stub target hello project (default)",
		TargetType:  "stub",
		Namespace:   "default",
		Image:       "hello:v1",
	},
	"web-stub": {
		ID:          "web-stub",
		Description: "Stub web service with PORT=8080 staged",
		TargetType:  "stub",
		Namespace:   "default",
		Image:       "web:v1",
		ServiceConfig: map[string]string{
			"PORT": "8080",
		},
	},
}

// DefaultRecipeID is used when `launchpad new <name>` omits a recipe id.
const DefaultRecipeID = "hello-stub"

// ListRecipes returns recipes sorted by id.
func ListRecipes() []Recipe {
	ids := make([]string, 0, len(builtinRecipes))
	for id := range builtinRecipes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Recipe, 0, len(ids))
	for _, id := range ids {
		out = append(out, builtinRecipes[id])
	}
	return out
}

// LookupRecipe returns a recipe by id.
func LookupRecipe(id string) (Recipe, bool) {
	r, ok := builtinRecipes[id]
	return r, ok
}

// NewArgs is the result of parsing `launchpad new` positional args.
type NewArgs struct {
	List    bool
	Recipe  string
	Project string
}

// ParseNewArgs implements the arg rules from the recipes design.
//
//  1. list → list mode
//  2. known recipe + name → recipe + project
//  3. known recipe, no name → error
//  4. unknown single arg → default recipe + project name
//  5. unknown + second → unknown recipe error
func ParseNewArgs(args []string) (NewArgs, error) {
	if len(args) == 0 {
		return NewArgs{}, fmt.Errorf("usage: launchpad new list | launchpad new [recipe] <project>")
	}
	if args[0] == "list" {
		if len(args) > 1 {
			return NewArgs{}, fmt.Errorf("usage: launchpad new list")
		}
		return NewArgs{List: true}, nil
	}
	if _, ok := LookupRecipe(args[0]); ok {
		if len(args) < 2 {
			return NewArgs{}, fmt.Errorf("project name required: launchpad new %s <name>", args[0])
		}
		if len(args) > 2 {
			return NewArgs{}, fmt.Errorf("usage: launchpad new [recipe] <project>")
		}
		return NewArgs{Recipe: args[0], Project: args[1]}, nil
	}
	if len(args) == 1 {
		return NewArgs{Recipe: DefaultRecipeID, Project: args[0]}, nil
	}
	if len(args) == 2 {
		return NewArgs{}, fmt.Errorf("unknown recipe %q; run \"launchpad new list\"", args[0])
	}
	return NewArgs{}, fmt.Errorf("usage: launchpad new list | launchpad new [recipe] <project>")
}

// ApplyRecipeOptions controls recipe application.
type ApplyRecipeOptions struct {
	TargetType string // empty → recipe default
	Namespace  string // empty → recipe default
	NoStage    bool
	Dir        string // where to write project-local config; empty → skip local write (use global)
	UseGlobal  bool   // also write ~/.launchpad/config (always true for day-one UX)
}

// ApplyRecipe creates a project and applies recipe staging + context.
func ApplyRecipe(ctx context.Context, client *apiclient.Client, recipe Recipe, projectName string, opts ApplyRecipeOptions) error {
	target := recipe.TargetType
	if opts.TargetType != "" {
		target = opts.TargetType
	}
	ns := recipe.Namespace
	if opts.Namespace != "" {
		ns = opts.Namespace
	}
	if ns == "" {
		ns = "default"
	}

	project, err := client.CreateProject(ctx, projectName, target, ns)
	if err != nil {
		return err
	}
	fmt.Printf("created project %s (recipe %s, target %s)\n", project.Name, recipe.ID, target)

	// Global + project-local context. Prefer project-only local so `env use`
	// is not permanently shadowed by a hardcoded local environment=dev.
	if err := saveActiveContext(project.Name, "dev"); err != nil {
		return fmt.Errorf("save context: %w", err)
	}
	if opts.Dir != "" {
		// Write project-local with project name only (env inherits default/global).
		if err := saveProjectLocalConfig(opts.Dir, localConfig{Project: project.Name}); err != nil {
			return fmt.Errorf("save project-local context: %w", err)
		}
	}
	fmt.Printf("context: %s @ dev\n", project.Name)

	if opts.NoStage {
		fmt.Println("next: launchpad image <ref> && launchpad deploy --wait")
		return nil
	}

	var changes []map[string]any
	var stagedParts []string
	if recipe.Image != "" {
		changes = append(changes, imageChange(recipe.Image))
		stagedParts = append(stagedParts, "image "+recipe.Image)
	}
	if len(recipe.ServiceConfig) > 0 {
		keys := make([]string, 0, len(recipe.ServiceConfig))
		for k := range recipe.ServiceConfig {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := recipe.ServiceConfig[k]
			changes = append(changes, map[string]any{
				"type":  "config",
				"key":   k,
				"value": v,
			})
			stagedParts = append(stagedParts, fmt.Sprintf("config %s=%s", k, v))
		}
	}
	if len(changes) > 0 {
		// Client must send Environment header for staging; caller wires it.
		if _, err := stage(ctx, client, project.Name, changes); err != nil {
			return err
		}
		fmt.Printf("staged: %s\n", strings.Join(stagedParts, ", "))
	}
	fmt.Println("next: launchpad diff && launchpad deploy --wait")
	return nil
}

func saveProjectLocalConfig(dir string, cfg localConfig) error {
	path := filepath.Join(dir, ".launchpad", "config")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func printRecipeList() {
	for _, r := range ListRecipes() {
		fmt.Printf("%-12s %s\n", r.ID, r.Description)
	}
}
