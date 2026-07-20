package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestListRecipesSorted(t *testing.T) {
	list := ListRecipes()
	if len(list) < 2 {
		t.Fatalf("expected at least hello-stub and web-stub, got %d", len(list))
	}
	for i := 1; i < len(list); i++ {
		if list[i-1].ID > list[i].ID {
			t.Fatalf("not sorted: %q after %q", list[i].ID, list[i-1].ID)
		}
	}
	if _, ok := LookupRecipe(DefaultRecipeID); !ok {
		t.Fatalf("default recipe %q missing", DefaultRecipeID)
	}
}

func TestParseNewArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    NewArgs
		wantErr bool
	}{
		{name: "list", args: []string{"list"}, want: NewArgs{List: true}},
		{name: "list extra", args: []string{"list", "x"}, wantErr: true},
		{name: "recipe name", args: []string{"hello-stub", "my-api"}, want: NewArgs{Recipe: "hello-stub", Project: "my-api"}},
		{name: "web recipe", args: []string{"web-stub", "app"}, want: NewArgs{Recipe: "web-stub", Project: "app"}},
		{name: "recipe no name", args: []string{"hello-stub"}, wantErr: true},
		{name: "default recipe", args: []string{"my-api"}, want: NewArgs{Recipe: DefaultRecipeID, Project: "my-api"}},
		{name: "unknown recipe", args: []string{"nope", "x"}, wantErr: true},
		{name: "empty", args: nil, wantErr: true},
		{name: "too many", args: []string{"hello-stub", "a", "b"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseNewArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %+v want %+v", got, tt.want)
			}
		})
	}
}

func TestSaveProjectLocalConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := localConfig{Project: "demo", Environment: "dev"}
	if err := saveProjectLocalConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".launchpad", "config"))
	if err != nil {
		t.Fatal(err)
	}
	var got localConfig
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got != cfg {
		t.Fatalf("got %+v want %+v", got, cfg)
	}
}

func TestWebStubHasPort(t *testing.T) {
	r, ok := LookupRecipe("web-stub")
	if !ok {
		t.Fatal("missing web-stub")
	}
	if r.ServiceConfig["PORT"] != "8080" {
		t.Fatalf("PORT: %q", r.ServiceConfig["PORT"])
	}
	if r.Image == "" {
		t.Fatal("expected image")
	}
}
