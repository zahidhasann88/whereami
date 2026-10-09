package sections

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/zahidhasann88/whereami/internal/env"
	"github.com/zahidhasann88/whereami/internal/system"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("testdata", "projects", name))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return p
}

func TestDetectProjectFixtures(t *testing.T) {
	fsys := system.OSFS{}
	cases := []struct {
		dir        string
		types      []string
		frameworks []string
		managers   []string
		name       string
	}{
		{"go-gin", []string{"Go"}, []string{"Gin"}, []string{"go modules"}, "example.com/api"},
		{"node-nextjs-ts", []string{"Node.js", "TypeScript"}, []string{"Next.js"}, []string{"pnpm"}, "web-app"},
		{"node-nest", []string{"Node.js"}, []string{"NestJS"}, []string{"npm"}, "billing"},
		{"node-express-js", []string{"Node.js"}, []string{"Express"}, []string{"yarn"}, "legacy-api"},
		{"python-django", []string{"Python"}, []string{"Django"}, []string{"uv"}, "shop"},
		{"python-fastapi", []string{"Python"}, []string{"FastAPI"}, []string{"pip"}, ""},
		{"python-flask", []string{"Python"}, []string{"Flask"}, []string{"pipenv"}, ""},
		{"rust-axum", []string{"Rust"}, []string{"Axum"}, []string{"cargo"}, "svc"},
		{"java-spring-maven", []string{"Java"}, []string{"Spring Boot"}, []string{"maven"}, ""},
		{"java-gradle", []string{"Java"}, []string{"Spring Boot"}, []string{"gradle"}, ""},
		{"php-laravel", []string{"PHP"}, []string{"Laravel"}, []string{"composer"}, "acme/portal"},
		{"ruby-rails", []string{"Ruby"}, []string{"Rails"}, []string{"bundler"}, ""},
		{"dotnet-aspnet", []string{".NET"}, []string{"ASP.NET Core"}, nil, ""},
		{"empty", nil, nil, nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.dir, func(t *testing.T) {
			d := detectProject(fsys, fixture(t, tc.dir))
			if !reflect.DeepEqual(nilIfEmpty(d.types), tc.types) {
				t.Errorf("types = %v, want %v", d.types, tc.types)
			}
			if !reflect.DeepEqual(nilIfEmpty(d.frameworks), tc.frameworks) {
				t.Errorf("frameworks = %v, want %v", d.frameworks, tc.frameworks)
			}
			if !reflect.DeepEqual(nilIfEmpty(d.packageManagers), tc.managers) {
				t.Errorf("package managers = %v, want %v", d.packageManagers, tc.managers)
			}
			if d.name != tc.name {
				t.Errorf("name = %q, want %q", d.name, tc.name)
			}
		})
	}
}

func nilIfEmpty(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return s
}

func TestDetectProjectManifests(t *testing.T) {
	d := detectProject(system.OSFS{}, fixture(t, "node-nextjs-ts"))
	want := []string{"package.json"}
	if !reflect.DeepEqual(d.manifests, want) {
		t.Errorf("manifests = %v, want %v", d.manifests, want)
	}
}

func TestFindRootNearestMarker(t *testing.T) {
	fsys := system.OSFS{}
	app := fixture(t, filepath.Join("nested", "app"))
	root, src := findRoot(fsys, filepath.Join(app, "src"))
	if root != app || src != "marker" {
		t.Errorf("findRoot = (%q, %q), want (%q, marker)", root, src, app)
	}
}

func TestFindRootGitMarker(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "tool"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Git worktrees and submodules use a .git file as their root marker.
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: /nowhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/tool\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, src := findRoot(system.OSFS{}, filepath.Join(dir, "cmd", "tool"))
	if root != dir || src != "git" {
		t.Errorf("findRoot = (%q, %q), want (%q, git)", root, src, dir)
	}
}

func TestFindRootNone(t *testing.T) {
	dir := t.TempDir()
	root, src := findRoot(system.OSFS{}, dir)
	// t.TempDir lives under the system temp directory, which has no markers.
	if root != "" || src != "" {
		t.Skipf("unexpected marker above %s: (%q, %q)", dir, root, src)
	}
}

func TestProjectSectionCollect(t *testing.T) {
	e := &env.Env{Dir: fixture(t, filepath.Join("nested", "app", "src")), FS: system.OSFS{}}
	res, err := Project{}.Collect(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	info, ok := res.Data.(ProjectInfo)
	if !ok {
		t.Fatalf("data type %T", res.Data)
	}
	if info.RootSource != "marker" || filepath.Base(info.Root) != "app" {
		t.Errorf("root = %q (%s)", info.Root, info.RootSource)
	}
	if info.Name != "nested-app" {
		t.Errorf("name = %q", info.Name)
	}
	if res.Status != StatusOK {
		t.Errorf("status = %s", res.Status)
	}
}

func TestProjectSectionUsesDirectoryWhenNoMarker(t *testing.T) {
	dir := t.TempDir()
	res, err := Project{}.Collect(context.Background(), &env.Env{Dir: dir, FS: system.OSFS{}})
	if err != nil {
		t.Fatal(err)
	}
	info := res.Data.(ProjectInfo)
	if info.RootSource == "marker" || info.RootSource == "git" {
		// Only meaningful when the temp dir has no marker above it.
		t.Skipf("marker found above temp dir")
	}
	if info.Root != dir || info.RootSource != "directory" {
		t.Errorf("root = %q (%s), want %q (directory)", info.Root, info.RootSource, dir)
	}
	if len(info.Types) != 0 {
		t.Errorf("types = %v, want none", info.Types)
	}
}

func TestDetectTypesSortedManifests(t *testing.T) {
	// Manifests are sorted for stable output.
	d := detectProject(system.OSFS{}, fixture(t, "node-nextjs-ts"))
	if !sort.StringsAreSorted(d.manifests) {
		t.Errorf("manifests not sorted: %v", d.manifests)
	}
}
