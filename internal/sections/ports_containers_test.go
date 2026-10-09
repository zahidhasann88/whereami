package sections

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zahidhasann88/whereami/internal/env"
	"github.com/zahidhasann88/whereami/internal/env/envtest"
	"github.com/zahidhasann88/whereami/internal/system"
)

func TestFilterAndGroupListeners(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "api")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	listeners := []env.Listener{
		{IP: "127.0.0.1", Port: 3000, Proto: "tcp", PID: 100, Process: "node", Cwd: inside},
		{IP: "::1", Port: 3000, Proto: "tcp", PID: 100, Process: "node", Cwd: inside},
		{IP: "0.0.0.0", Port: 5432, Proto: "tcp", PID: 200, Process: "postgres", Cwd: outside},
		{IP: "0.0.0.0", Port: 8080, Proto: "tcp", PID: 300, Process: "root-owned", CwdErr: errors.New("permission denied")},
		{IP: "127.0.0.1", Port: 4000, Proto: "tcp", PID: 400, Process: "go", Cwd: root},
	}
	var inaccessible int
	mine := FilterListeners(listeners, root, &inaccessible)
	if inaccessible != 1 {
		t.Errorf("inaccessible = %d, want 1", inaccessible)
	}
	ports := GroupPorts(mine)
	if len(ports) != 2 {
		t.Fatalf("ports = %+v", ports)
	}
	if ports[0].Port != 3000 || !reflect.DeepEqual(ports[0].Addresses, []string{"127.0.0.1", "::1"}) {
		t.Errorf("first port = %+v", ports[0])
	}
	if ports[1].Port != 4000 {
		t.Errorf("second port = %+v", ports[1])
	}
}

func TestPortsSectionCollect(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name       string
		procs      env.ProcessSource
		wantStatus Status
		wantPorts  int
	}{
		{"owned by project", envtest.Processes{Items: []env.Listener{
			{IP: "127.0.0.1", Port: 3000, Proto: "tcp", PID: 7, Process: "node", Cwd: root},
		}}, StatusOK, 1},
		{"nothing in project", envtest.Processes{Items: []env.Listener{
			{IP: "0.0.0.0", Port: 22, Proto: "tcp", PID: 1, Process: "sshd", Cwd: "/"},
		}}, StatusOK, 0},
		{"every cwd unreadable", envtest.Processes{Items: []env.Listener{
			{IP: "0.0.0.0", Port: 22, Proto: "tcp", PID: 1, CwdErr: errors.New("access denied")},
		}}, StatusUnavailable, 0},
		{"listing fails", envtest.Processes{Err: errors.New("boom")}, StatusUnavailable, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Ports{}.Collect(context.Background(), &env.Env{Dir: root, FS: system.OSFS{}, Processes: tc.procs})
			if err != nil {
				t.Fatal(err)
			}
			if res.Status != tc.wantStatus {
				t.Fatalf("status = %s (%s), want %s", res.Status, res.Reason, tc.wantStatus)
			}
			if tc.wantStatus == StatusOK && len(res.Data.(PortsInfo).Listening) != tc.wantPorts {
				t.Errorf("ports = %+v", res.Data)
			}
		})
	}
}

func loadFixtureContainers(t *testing.T) []APIContainer {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "docker", "containers.json"))
	if err != nil {
		t.Fatal(err)
	}
	list, err := ParseContainers(raw)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func TestMatchContainersByLabels(t *testing.T) {
	list := loadFixtureContainers(t)
	root := "/work/shop"
	got := MatchContainers(list, root, map[string]bool{"shop": true})
	var names []string
	for _, c := range got {
		names = append(names, c.Name+"("+c.MatchedBy+")")
	}
	// shop-web and shop-db share the working dir. other-shop-web lives in a
	// different directory despite the same project name, so it is excluded.
	// legacy-job has only a project label, so it matches by compose project.
	// unrelated-redis carries no compose labels at all.
	want := []string{
		"legacy-job(compose_project)",
		"shop-db-1(working_dir)",
		"shop-web-1(working_dir)",
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("matched = %v\nwant      %v", names, want)
	}
}

func TestMatchContainersProjectNameWithoutWorkingDirMatchesOnlyKnownProjects(t *testing.T) {
	list := loadFixtureContainers(t)
	got := MatchContainers(list, "/somewhere/else", map[string]bool{"billing": true})
	if len(got) != 0 {
		t.Errorf("expected no matches, got %+v", got)
	}
}

func TestContainerPortFormatting(t *testing.T) {
	list := loadFixtureContainers(t)
	got := MatchContainers(list, "/work/shop", map[string]bool{"shop": true})
	for _, c := range got {
		if c.Name == "shop-web-1" {
			if !reflect.DeepEqual(c.Ports, []string{"0.0.0.0:8080->8000/tcp"}) {
				t.Errorf("ports = %v", c.Ports)
			}
			if c.ID != "9f1c2a7b3d4e" || c.Service != "web" || c.Project != "shop" {
				t.Errorf("container = %+v", c)
			}
		}
		if c.Name == "shop-db-1" && !reflect.DeepEqual(c.Ports, []string{"5432/tcp"}) {
			t.Errorf("unpublished ports = %v", c.Ports)
		}
	}
}

func TestParseComposeServicesAndName(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("testdata", "compose", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	services := ParseComposeServices(string(content))
	if !reflect.DeepEqual(services, []string{"web", "db", "worker"}) {
		t.Errorf("services = %v", services)
	}
	if got := ComposeProjectName(string(content), "fallback"); got != "fallback" {
		t.Errorf("project name = %q, want fallback", got)
	}
	if got := ComposeProjectName("name: My_Shop\nservices:\n  a:\n    image: x\n", "fallback"); got != "my_shop" {
		t.Errorf("explicit name = %q", got)
	}
	if got := DefaultComposeProject("/work/My Shop.v2"); got != "myshopv2" {
		t.Errorf("default project = %q", got)
	}
}

func TestContainersCollectDockerAndCompose(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "docker", "containers.json"))
	if err != nil {
		t.Fatal(err)
	}
	dir := writeFiles(t, map[string]string{
		"docker-compose.yml": "name: shop\nservices:\n  web:\n    image: x\n",
	})
	withDocker := &env.Env{Dir: dir, FS: system.OSFS{}, Docker: envtest.Docker{
		Responses: map[string][]byte{"/containers/json": raw},
	}}
	res, _ := Containers{}.Collect(context.Background(), withDocker)
	info := res.Data.(ContainersInfo)
	if !info.DockerAvailable || len(info.ComposeFiles) != 1 || info.ComposeFiles[0].Project != "shop" {
		t.Errorf("info = %+v", info)
	}
	if len(info.ComposeFiles[0].Services) != 1 {
		t.Errorf("services = %v", info.ComposeFiles[0].Services)
	}

	// Docker unreachable with a compose file present: still OK, not an error.
	noDocker := &env.Env{Dir: dir, FS: system.OSFS{}, Docker: envtest.Docker{Err: errors.New("no socket")}}
	res, _ = Containers{}.Collect(context.Background(), noDocker)
	if res.Status != StatusOK || res.Data.(ContainersInfo).DockerAvailable {
		t.Errorf("result = %+v", res)
	}

	// Docker unreachable and no compose file: skipped silently.
	empty := t.TempDir()
	res, _ = Containers{}.Collect(context.Background(), &env.Env{Dir: empty, FS: system.OSFS{}, Docker: envtest.Docker{Err: errors.New("no socket")}})
	if res.Status != StatusSkipped {
		t.Errorf("status = %s, want skipped", res.Status)
	}
	// Docker disabled (nil client) and no compose file: also skipped.
	res, _ = Containers{}.Collect(context.Background(), &env.Env{Dir: empty, FS: system.OSFS{}})
	if res.Status != StatusSkipped {
		t.Errorf("nil docker status = %s", res.Status)
	}
}

func TestParseContainersRejectsGarbage(t *testing.T) {
	if _, err := ParseContainers([]byte("{not json")); err == nil {
		t.Error("expected decode error")
	}
}

const secretValue = "hunter2-DO-NOT-LEAK-9f8e7d"

func configFixture(t *testing.T) string {
	return writeFiles(t, map[string]string{
		".env":                     "# comment\nDATABASE_URL=postgres://user:" + secretValue + "@db/app\nexport API_KEY=\"" + secretValue + "\"\n\nDEBUG=1\n",
		".env.local":               "TOKEN=" + secretValue + "\n",
		".env.example":             "DATABASE_URL=postgres://user:changeme@localhost/app\n",
		"config.yaml":              "password: " + secretValue + "\n",
		"docker-compose.yml":       "services:\n  web:\n    image: x\n",
		"Dockerfile":               "FROM scratch\n",
		"Makefile":                 "build:\n\tgo build\n",
		".github/workflows/ci.yml": "name: ci\n",
	})
}

func TestConfigEnvFilesCountOnlyAndNeverLeak(t *testing.T) {
	dir := configFixture(t)
	e := &env.Env{Dir: dir, FS: system.OSFS{}, Runner: envtest.NewRunner(map[string]envtest.Reply{
		"git ls-files -- .env":       {Err: errors.New("exit status 128")},
		"git ls-files -- .env.local": {Err: errors.New("exit status 128")},
	})}
	res, err := Config{}.Collect(context.Background(), e)
	if err != nil {
		t.Fatal(err)
	}
	info := res.Data.(ConfigInfo)
	counts := map[string]int{}
	for _, f := range info.EnvFiles {
		counts[f.Name] = f.Variables
		if len(f.Keys) != 0 {
			t.Errorf("keys leaked without --show-env-keys: %+v", f)
		}
	}
	if counts[".env"] != 3 || counts[".env.local"] != 1 || counts[".env.example"] != 1 {
		t.Errorf("variable counts = %v", counts)
	}
	for _, f := range info.EnvFiles {
		if f.Name == ".env.example" && !f.Template {
			t.Error(".env.example should be a template")
		}
	}
	if len(info.Files) == 0 {
		t.Error("expected non-env config files")
	}
}

func TestConfigShowEnvKeysListsNamesOnly(t *testing.T) {
	dir := configFixture(t)
	e := &env.Env{Dir: dir, FS: system.OSFS{}, ShowEnvKeys: true, Runner: envtest.NewRunner(nil)}
	res, _ := Config{}.Collect(context.Background(), e)
	info := res.Data.(ConfigInfo)
	for _, f := range info.EnvFiles {
		if f.Name == ".env" {
			want := []string{"DATABASE_URL", "API_KEY", "DEBUG"}
			if !reflect.DeepEqual(f.Keys, want) {
				t.Errorf("keys = %v, want %v", f.Keys, want)
			}
		}
		for _, k := range f.Keys {
			if strings.Contains(k, secretValue) {
				t.Errorf("secret leaked as key %q", k)
			}
		}
	}
}

func TestConfigWarnsWhenEnvNotGitignored(t *testing.T) {
	dir := configFixture(t)
	// Simulate git: .env is not ignored (exit 1 from check-ignore), .env.local is.
	runner := envtest.NewRunner(map[string]envtest.Reply{
		"git ls-files -- .env":                              {},
		"git check-ignore --quiet --no-index -- .env":       {Err: exitStatus(1)},
		"git ls-files -- .env.local":                        {},
		"git check-ignore --quiet --no-index -- .env.local": {},
	})
	res, _ := Config{}.Collect(context.Background(), &env.Env{Dir: dir, FS: system.OSFS{}, Runner: runner})
	info := res.Data.(ConfigInfo)
	warned := info.Warnings
	if len(warned) != 1 || !strings.Contains(warned[0], ".env is not gitignored") {
		t.Errorf("warnings = %v", warned)
	}
	for _, f := range info.EnvFiles {
		if f.Name == ".env.local" && f.Gitignore != GitignoreIgnored {
			t.Errorf(".env.local gitignore = %s", f.Gitignore)
		}
	}
}

func TestConfigWarnsWhenEnvTracked(t *testing.T) {
	dir := configFixture(t)
	runner := envtest.NewRunner(map[string]envtest.Reply{
		"git ls-files -- .env":                              {Stdout: ".env\n"},
		"git ls-files -- .env.local":                        {},
		"git check-ignore --quiet --no-index -- .env.local": {},
	})
	res, _ := Config{}.Collect(context.Background(), &env.Env{Dir: dir, FS: system.OSFS{}, Runner: runner})
	info := res.Data.(ConfigInfo)
	found := false
	for _, w := range info.Warnings {
		if strings.Contains(w, ".env is tracked by git") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected tracked warning, got %v", info.Warnings)
	}
}

func TestConfigNoWarningOutsideGit(t *testing.T) {
	dir := configFixture(t)
	runner := envtest.NewRunner(map[string]envtest.Reply{
		"git ls-files -- .env":       {Err: errors.New("exit status 128")},
		"git ls-files -- .env.local": {Err: errors.New("exit status 128")},
	})
	res, _ := Config{}.Collect(context.Background(), &env.Env{Dir: dir, FS: system.OSFS{}, Runner: runner})
	info := res.Data.(ConfigInfo)
	if len(info.Warnings) != 0 {
		t.Errorf("warnings outside git = %v", info.Warnings)
	}
}

func TestEnvFileValueNeverInRenderedJSON(t *testing.T) {
	dir := configFixture(t)
	res, _ := Config{}.Collect(context.Background(), &env.Env{Dir: dir, FS: system.OSFS{}, ShowEnvKeys: true, Runner: envtest.NewRunner(nil)})
	b, err := jsonMarshal(res.Data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), secretValue) {
		t.Fatalf("secret value present in data: %s", b)
	}
}

// exitStatus returns an error that behaves like *exec.ExitError with the given code.
func exitStatus(code int) error {
	return exitCodeErr(code)
}
