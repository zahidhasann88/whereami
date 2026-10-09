package sections

import (
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/zahidhasann88/whereami/internal/env"
)

// Project reports the project root, detected languages and frameworks.
type Project struct{}

func (Project) Name() string { return "project" }

// ProjectInfo is the data of the project section.
type ProjectInfo struct {
	Dir             string   `json:"dir"`
	Root            string   `json:"root"`
	RootSource      string   `json:"root_source"`
	Name            string   `json:"name,omitempty"`
	Types           []string `json:"types"`
	Frameworks      []string `json:"frameworks"`
	PackageManagers []string `json:"package_managers"`
	Manifests       []string `json:"manifests"`
}

func (Project) Collect(ctx context.Context, e *env.Env) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	info := ProjectInfo{Dir: e.Dir}
	root, source := findRoot(e.FS, e.Dir)
	if root == "" {
		root, source = e.Dir, "directory"
	}
	info.Root, info.RootSource = root, source

	d := detectProject(e.FS, root)
	info.Name = d.name
	info.Types = nonNil(d.types)
	info.Frameworks = nonNil(d.frameworks)
	info.PackageManagers = nonNil(d.packageManagers)
	info.Manifests = nonNil(d.manifests)
	return Result{Status: StatusOK, Data: info}, nil
}

// projectDetection is shared by project and runtime collection.
type projectDetection struct {
	name            string
	types           []string
	frameworks      []string
	packageManagers []string
	manifests       []string
}

type frameworkRule struct {
	needle string // substring searched for in the manifest
	name   string
}

var goFrameworks = []frameworkRule{
	{"github.com/gin-gonic/gin", "Gin"},
	{"github.com/labstack/echo", "Echo"},
	{"github.com/gofiber/fiber", "Fiber"},
	{"github.com/go-chi/chi", "chi"},
}

var nodeFrameworks = []struct{ dep, name string }{
	{"next", "Next.js"},
	{"@nestjs/core", "NestJS"},
	{"express", "Express"},
	{"fastify", "Fastify"},
	{"nuxt", "Nuxt"},
	{"@sveltejs/kit", "SvelteKit"},
	{"@angular/core", "Angular"},
	{"vue", "Vue"},
}

var (
	goModuleRe     = regexp.MustCompile(`(?m)^module\s+(\S+)`)
	tomlNameRe     = regexp.MustCompile(`(?m)^name\s*=\s*"([^"]+)"`)
	pyFrameworkRe  = regexp.MustCompile(`(?im)(?:^|[\s"'\[,])(django|fastapi|flask)\s*(?:["'=<>!~\[;,\]]|$)`)
	rustFrameworks = regexp.MustCompile(`(?m)^\s*(axum|actix-web|rocket)\s*=`)
	rubyRailsRe    = regexp.MustCompile(`(?m)^\s*gem\s+['"]rails['"]`)
)

var pythonManifests = []string{"pyproject.toml", "requirements.txt", "Pipfile", "setup.py"}

// lockfilePMs maps lockfile names to Node package managers.
var lockfilePMs = []struct{ file, pm string }{
	{"pnpm-lock.yaml", "pnpm"},
	{"yarn.lock", "yarn"},
	{"bun.lockb", "bun"},
	{"bun.lock", "bun"},
	{"package-lock.json", "npm"},
	{"npm-shrinkwrap.json", "npm"},
}

// detectProject inspects the files directly inside root.
func detectProject(fsys env.FS, root string) projectDetection {
	names := listNames(fsys, root)
	read := func(f string) string {
		s, _ := readText(fsys, filepath.Join(root, f))
		return s
	}
	var d projectDetection
	addUnique := func(list *[]string, v string) {
		for _, x := range *list {
			if x == v {
				return
			}
		}
		*list = append(*list, v)
	}

	// Go
	if names["go.mod"] {
		content := read("go.mod")
		addUnique(&d.manifests, "go.mod")
		addUnique(&d.types, "Go")
		addUnique(&d.packageManagers, "go modules")
		if m := goModuleRe.FindStringSubmatch(content); m != nil {
			d.name = m[1]
		}
		for _, fw := range goFrameworks {
			if strings.Contains(content, fw.needle) {
				addUnique(&d.frameworks, fw.name)
			}
		}
	}

	// Node.js / TypeScript
	if names["package.json"] {
		addUnique(&d.manifests, "package.json")
		addUnique(&d.types, "Node.js")
		var pkg struct {
			Name            string            `json:"name"`
			PackageManager  string            `json:"packageManager"`
			Dependencies    map[string]string `json:"dependencies"`
			DevDependencies map[string]string `json:"devDependencies"`
		}
		if raw := read("package.json"); raw != "" {
			if err := json.Unmarshal([]byte(raw), &pkg); err == nil {
				if d.name == "" {
					d.name = pkg.Name
				}
				if pm, _, ok := strings.Cut(pkg.PackageManager, "@"); ok && pm != "" {
					addUnique(&d.packageManagers, pm)
				}
				deps := map[string]bool{}
				for k := range pkg.Dependencies {
					deps[k] = true
				}
				for k := range pkg.DevDependencies {
					deps[k] = true
				}
				for _, fw := range nodeFrameworks {
					if deps[fw.dep] {
						addUnique(&d.frameworks, fw.name)
					}
				}
				if names["tsconfig.json"] || deps["typescript"] {
					addUnique(&d.types, "TypeScript")
				}
			}
		}
		for _, lp := range lockfilePMs {
			if names[lp.file] {
				addUnique(&d.packageManagers, lp.pm)
			}
		}
	}

	// Python
	var pyText strings.Builder
	pyFound := false
	for _, f := range pythonManifests {
		if names[f] {
			pyFound = true
			addUnique(&d.manifests, f)
			pyText.WriteString(read(f))
			pyText.WriteString("\n")
		}
	}
	if pyFound {
		addUnique(&d.types, "Python")
		if m := tomlNameRe.FindStringSubmatch(read("pyproject.toml")); m != nil && d.name == "" {
			d.name = m[1]
		}
		for _, m := range pyFrameworkRe.FindAllStringSubmatch(pyText.String(), -1) {
			addUnique(&d.frameworks, pyFrameworkName(strings.ToLower(m[1])))
		}
		for _, lp := range []struct{ file, pm string }{
			{"uv.lock", "uv"}, {"poetry.lock", "poetry"}, {"Pipfile", "pipenv"},
		} {
			if names[lp.file] {
				addUnique(&d.packageManagers, lp.pm)
			}
		}
		if names["requirements.txt"] && len(d.packageManagers) == 0 {
			addUnique(&d.packageManagers, "pip")
		}
	}

	// Rust
	if names["Cargo.toml"] {
		content := read("Cargo.toml")
		addUnique(&d.manifests, "Cargo.toml")
		addUnique(&d.types, "Rust")
		addUnique(&d.packageManagers, "cargo")
		if m := tomlNameRe.FindStringSubmatch(content); m != nil && d.name == "" {
			d.name = m[1]
		}
		for _, m := range rustFrameworks.FindAllStringSubmatch(content, -1) {
			switch m[1] {
			case "axum":
				addUnique(&d.frameworks, "Axum")
			case "actix-web":
				addUnique(&d.frameworks, "Actix Web")
			case "rocket":
				addUnique(&d.frameworks, "Rocket")
			}
		}
	}

	// Java (Maven and Gradle)
	javaText := ""
	if names["pom.xml"] {
		addUnique(&d.manifests, "pom.xml")
		addUnique(&d.packageManagers, "maven")
		javaText += read("pom.xml")
	}
	for _, g := range []string{"build.gradle", "build.gradle.kts"} {
		if names[g] {
			addUnique(&d.manifests, g)
			addUnique(&d.packageManagers, "gradle")
			javaText += read(g)
		}
	}
	if javaText != "" {
		addUnique(&d.types, "Java")
		if strings.Contains(javaText, "spring-boot") || strings.Contains(javaText, "org.springframework.boot") {
			addUnique(&d.frameworks, "Spring Boot")
		}
	}

	// PHP
	if names["composer.json"] {
		addUnique(&d.manifests, "composer.json")
		addUnique(&d.types, "PHP")
		addUnique(&d.packageManagers, "composer")
		var comp struct {
			Name    string            `json:"name"`
			Require map[string]string `json:"require"`
		}
		if raw := read("composer.json"); raw != "" && json.Unmarshal([]byte(raw), &comp) == nil {
			if d.name == "" {
				d.name = comp.Name
			}
			if _, ok := comp.Require["laravel/framework"]; ok {
				addUnique(&d.frameworks, "Laravel")
			}
			if _, ok := comp.Require["symfony/framework-bundle"]; ok {
				addUnique(&d.frameworks, "Symfony")
			}
		}
	}

	// Ruby
	if names["Gemfile"] {
		addUnique(&d.manifests, "Gemfile")
		addUnique(&d.types, "Ruby")
		addUnique(&d.packageManagers, "bundler")
		if rubyRailsRe.MatchString(read("Gemfile")) {
			addUnique(&d.frameworks, "Rails")
		}
	}

	// .NET
	var csproj []string
	for n := range names {
		if strings.HasSuffix(n, ".csproj") {
			csproj = append(csproj, n)
		}
	}
	if len(csproj) > 0 {
		sort.Strings(csproj)
		addUnique(&d.types, ".NET")
		for _, c := range csproj {
			addUnique(&d.manifests, c)
		}
		if strings.Contains(read(csproj[0]), "Microsoft.AspNetCore") {
			addUnique(&d.frameworks, "ASP.NET Core")
		}
	}

	sort.Strings(d.manifests)
	return d
}

func pyFrameworkName(lower string) string {
	switch lower {
	case "django":
		return "Django"
	case "fastapi":
		return "FastAPI"
	case "flask":
		return "Flask"
	}
	return lower
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
