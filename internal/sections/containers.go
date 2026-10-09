package sections

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/zahidhasann88/whereami/internal/env"
)

// Containers reports Compose files and running project containers.
type Containers struct{}

func (Containers) Name() string { return "containers" }

// Labels set by Docker Compose on the containers it creates.
const (
	labelComposeProject    = "com.docker.compose.project"
	labelComposeService    = "com.docker.compose.service"
	labelComposeWorkingDir = "com.docker.compose.project.working_dir"
)

// ContainersInfo is the data of the containers section.
type ContainersInfo struct {
	DockerAvailable bool            `json:"docker_available"`
	ComposeFiles    []ComposeFile   `json:"compose_files"`
	Running         []ContainerInfo `json:"running"`
}

// ComposeFile is a Compose file found in the project root.
type ComposeFile struct {
	Path     string   `json:"path"`
	Project  string   `json:"project"`
	Services []string `json:"services"`
}

// ContainerInfo is a running container that belongs to the project.
type ContainerInfo struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Image     string   `json:"image"`
	State     string   `json:"state"`
	Status    string   `json:"status"`
	Project   string   `json:"compose_project,omitempty"`
	Service   string   `json:"compose_service,omitempty"`
	Ports     []string `json:"ports"`
	MatchedBy string   `json:"matched_by"`
}

// APIContainer is one element of the Docker Engine GET /containers/json response.
type APIContainer struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Image  string            `json:"Image"`
	State  string            `json:"State"`
	Status string            `json:"Status"`
	Labels map[string]string `json:"Labels"`
	Ports  []APIPort         `json:"Ports"`
}

// APIPort is a port mapping as reported by the Docker Engine API.
type APIPort struct {
	IP          string `json:"IP"`
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
}

var composeFileRe = regexp.MustCompile(`^(docker-compose|compose)(\.[^/]+)?\.ya?ml$`)

func (Containers) Collect(ctx context.Context, e *env.Env) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	root, _ := findRoot(e.FS, e.Dir)
	if root == "" {
		root = e.Dir
	}
	compose := listComposeFiles(e.FS, root)
	projects := map[string]bool{}
	for _, c := range compose {
		projects[c.Project] = true
	}

	info := ContainersInfo{ComposeFiles: compose}
	if e.Docker != nil {
		raw, err := e.Docker.Get(ctx, "/containers/json")
		if err == nil {
			var list []APIContainer
			if jerr := json.Unmarshal(raw, &list); jerr == nil {
				info.DockerAvailable = true
				info.Running = MatchContainers(list, root, projects)
			}
		}
	}
	if !info.DockerAvailable && len(compose) == 0 {
		return Result{Status: StatusSkipped}, nil
	}
	if info.ComposeFiles == nil {
		info.ComposeFiles = []ComposeFile{}
	}
	if info.Running == nil {
		info.Running = []ContainerInfo{}
	}
	return Result{Status: StatusOK, Data: info}, nil
}

// MatchContainers matches working-directory labels, falling back to Compose project labels only when absent.
func MatchContainers(list []APIContainer, root string, projects map[string]bool) []ContainerInfo {
	var out []ContainerInfo
	for _, c := range list {
		wd := c.Labels[labelComposeWorkingDir]
		proj := c.Labels[labelComposeProject]
		var matched string
		switch {
		case wd != "":
			if within(wd, root) {
				matched = "working_dir"
			}
		case proj != "" && projects[proj]:
			matched = "compose_project"
		}
		if matched == "" {
			continue
		}
		out = append(out, ContainerInfo{
			ID:        shortID(c.ID),
			Name:      containerName(c),
			Image:     c.Image,
			State:     c.State,
			Status:    c.Status,
			Project:   proj,
			Service:   c.Labels[labelComposeService],
			Ports:     formatPorts(c.Ports),
			MatchedBy: matched,
		})
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}

// ParseContainers decodes a GET /containers/json body.
func ParseContainers(raw []byte) ([]APIContainer, error) {
	var list []APIContainer
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("decoding container list: %w", err)
	}
	return list, nil
}

func listComposeFiles(fsys env.FS, root string) []ComposeFile {
	entries, err := fsys.ReadDir(root)
	if err != nil {
		return nil
	}
	var names []string
	for _, en := range entries {
		if !en.IsDir() && composeFileRe.MatchString(en.Name()) {
			names = append(names, en.Name())
		}
	}
	sort.Strings(names)
	defaultProject := DefaultComposeProject(root)
	var out []ComposeFile
	for _, n := range names {
		content, _ := readText(fsys, filepath.Join(root, n))
		proj := ComposeProjectName(content, defaultProject)
		out = append(out, ComposeFile{
			Path:     n,
			Project:  proj,
			Services: ParseComposeServices(content),
		})
	}
	return out
}

var (
	composeTopNameRe = regexp.MustCompile(`(?m)^name:\s*["']?([A-Za-z0-9_-]+)["']?\s*(#.*)?$`)
	composeServiceRe = regexp.MustCompile(`^ {2}([A-Za-z0-9_.-]+):\s*(#.*)?$`)
)

// ComposeProjectName reads top-level name, using fallback when absent.
func ComposeProjectName(content, fallback string) string {
	if m := composeTopNameRe.FindStringSubmatch(content); m != nil {
		return normalizeProjectName(m[1])
	}
	return fallback
}

// DefaultComposeProject derives the compose project name from the directory
// containing the compose file, following Compose's normalisation rules.
func DefaultComposeProject(dir string) string {
	return normalizeProjectName(filepath.Base(dir))
}

func normalizeProjectName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ParseComposeServices reads service names with a limited line-based parser.
func ParseComposeServices(content string) []string {
	var out []string
	inServices := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") {
			inServices = strings.HasPrefix(line, "services:")
			continue
		}
		if !inServices {
			continue
		}
		if m := composeServiceRe.FindStringSubmatch(line); m != nil {
			out = append(out, m[1])
		}
	}
	return out
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func containerName(c APIContainer) string {
	if len(c.Names) > 0 {
		return strings.TrimPrefix(c.Names[0], "/")
	}
	return shortID(c.ID)
}

func formatPorts(ports []APIPort) []string {
	out := []string{}
	for _, p := range ports {
		s := fmt.Sprintf("%d/%s", p.PrivatePort, p.Type)
		if p.PublicPort > 0 {
			ip := p.IP
			if ip == "" {
				ip = "0.0.0.0"
			}
			s = fmt.Sprintf("%s:%d->%s", ip, p.PublicPort, s)
		}
		out = append(out, s)
	}
	return out
}
