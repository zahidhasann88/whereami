package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/zahidhasann88/whereami/internal/sections"
)

// Text writes the human-readable summary. Skipped sections are omitted.
func Text(w io.Writer, rep Report, st Style) error {
	sym := st.symbols()
	var b strings.Builder
	title := fmt.Sprintf("whereami %s %s %s", rep.Version, sym.dash, rep.Path)
	b.WriteString(st.bold(title))
	b.WriteString("\n")

	for _, r := range rep.Results {
		if r.Status == sections.StatusSkipped {
			continue
		}
		b.WriteString("\n")
		b.WriteString(st.bold(sectionTitle(r.Name)))
		b.WriteString("\n")
		if r.Status != sections.StatusOK {
			msg := r.Reason
			if r.Status == sections.StatusTimedOut {
				b.WriteString("  " + st.yellow(msg) + "\n")
				continue
			}
			b.WriteString("  " + st.dim("unavailable "+sym.dash+" "+msg) + "\n")
			continue
		}
		switch d := r.Data.(type) {
		case sections.ProjectInfo:
			writeProject(&b, st, d)
		case sections.GitInfo:
			writeGit(&b, st, sym, d)
		case sections.RuntimeData:
			writeRuntime(&b, st, sym, d)
		case sections.PortsInfo:
			writePorts(&b, st, d)
		case sections.ContainersInfo:
			writeContainers(&b, st, sym, d)
		case sections.ConfigInfo:
			writeConfig(&b, st, sym, d)
		default:
			b.WriteString("  " + st.dim("no details") + "\n")
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func sectionTitle(name string) string {
	switch name {
	case "project":
		return "Project"
	case "git":
		return "Git"
	case "runtime":
		return "Runtime"
	case "ports":
		return "Ports"
	case "containers":
		return "Containers"
	case "config":
		return "Config"
	}
	return name
}

func writeProject(b *strings.Builder, st Style, d sections.ProjectInfo) {
	var rootNote string
	switch d.RootSource {
	case "git":
		rootNote = "git root"
	case "marker":
		rootNote = "project marker"
	default:
		rootNote = "no marker found, using directory"
	}
	line(b, "Path", d.Dir)
	line(b, "Root", fmt.Sprintf("%s %s", d.Root, st.dim("("+rootNote+")")))
	if d.Name != "" {
		line(b, "Name", d.Name)
	}
	line(b, "Types", joinOrDash(d.Types))
	if len(d.Frameworks) > 0 {
		line(b, "Frameworks", strings.Join(d.Frameworks, ", "))
	}
	if len(d.PackageManagers) > 0 {
		line(b, "Managers", strings.Join(d.PackageManagers, ", "))
	}
	if len(d.Manifests) > 0 {
		line(b, "Manifests", strings.Join(d.Manifests, ", "))
	}
}

func writeGit(b *strings.Builder, st Style, sym symbols, d sections.GitInfo) {
	switch {
	case d.Detached:
		head := d.Head
		if head == "" {
			head = "no commits"
		}
		line(b, "Branch", "detached at "+head)
	case d.Unborn:
		line(b, "Branch", fmt.Sprintf("%s %s", d.Branch, st.dim("(no commits yet)")))
	default:
		branch := d.Branch
		if d.Upstream != "" {
			sync := ""
			switch {
			case d.Ahead == 0 && d.Behind == 0:
				sync = "up to date"
			default:
				var parts []string
				if d.Ahead > 0 {
					parts = append(parts, fmt.Sprintf("ahead %d", d.Ahead))
				}
				if d.Behind > 0 {
					parts = append(parts, fmt.Sprintf("behind %d", d.Behind))
				}
				sync = strings.Join(parts, ", ")
			}
			branch = fmt.Sprintf("%s %s %s %s", branch, sym.arrow, d.Upstream, st.dim("("+sync+")"))
		} else {
			branch = fmt.Sprintf("%s %s", branch, st.dim("(no upstream)"))
		}
		line(b, "Branch", branch)
	}

	if d.Clean {
		line(b, "State", st.green(sym.ok)+" clean")
	} else {
		var parts []string
		if d.Conflicted > 0 {
			parts = append(parts, fmt.Sprintf("%d conflicted", d.Conflicted))
		}
		parts = append(parts, fmt.Sprintf("%d staged", d.Staged), fmt.Sprintf("%d modified", d.Modified), fmt.Sprintf("%d untracked", d.Untracked))
		line(b, "State", st.yellow("dirty")+" "+st.dim(sym.dot)+" "+strings.Join(parts, ", "))
	}

	if c := d.LastCommit; c != nil {
		line(b, "Last commit", fmt.Sprintf("%s %q %s %s by %s", c.Hash, c.Subject, st.dim(sym.dot), c.RelativeTime, c.Author))
	} else if d.Unborn {
		line(b, "Last commit", st.dim("none yet"))
	}

	if d.Remote != nil {
		line(b, "Remote", fmt.Sprintf("%s %s", d.Remote.URL, st.dim("("+d.Remote.Name+")")))
	} else {
		line(b, "Remote", st.dim("none configured"))
	}
	line(b, "Stashes", fmt.Sprintf("%d", d.Stashes))
}

func writeRuntime(b *strings.Builder, st Style, sym symbols, d sections.RuntimeData) {
	if len(d.Runtimes) == 0 {
		b.WriteString("  " + st.dim("no runtimes relevant to this directory") + "\n")
		return
	}
	for _, r := range d.Runtimes {
		version := r.Version
		if version == "" {
			version = st.dim(r.Reason)
		}
		if len(r.Pins) == 0 {
			fmt.Fprintf(b, "  %-9s %s\n", r.Name, version)
			continue
		}
		for i, p := range r.Pins {
			name := r.Name
			ver := version
			if i > 0 {
				name, ver = "", ""
			}
			fmt.Fprintf(b, "  %-9s %-18s %s\n", name, ver, pinText(st, sym, p))
		}
	}
}

func pinText(st Style, sym symbols, p sections.PinInfo) string {
	base := fmt.Sprintf("%s by %s", p.Expr, p.Source)
	switch p.Status {
	case sections.PinMatch:
		return st.green(sym.ok) + " pinned " + base
	case sections.PinMismatch:
		return st.yellow(sym.warn) + " pinned " + base + " " + st.yellow(sym.dash+" mismatch")
	case sections.PinMissing:
		return st.yellow(sym.warn) + " pinned " + base + " " + st.yellow(sym.dash+" not installed")
	default:
		return st.dim(sym.unknown + " pinned " + base + " (cannot compare)")
	}
}

func writePorts(b *strings.Builder, st Style, d sections.PortsInfo) {
	if len(d.Listening) == 0 {
		b.WriteString("  " + st.dim("nothing listening inside this project") + "\n")
	}
	for _, p := range d.Listening {
		fmt.Fprintf(b, "  :%-6d %-10s pid %-7d %s\n", p.Port, p.Process, p.PID, strings.Join(p.Addresses, ", "))
	}
	if d.Inaccessible > 0 {
		b.WriteString("  " + st.dim(fmt.Sprintf("%d other listener(s) had unreadable working directories and were not checked", d.Inaccessible)) + "\n")
	}
}

func writeContainers(b *strings.Builder, st Style, sym symbols, d sections.ContainersInfo) {
	for _, cf := range d.ComposeFiles {
		desc := fmt.Sprintf("project %s", cf.Project)
		if len(cf.Services) > 0 {
			desc += "; services " + strings.Join(cf.Services, ", ")
		}
		line(b, "Compose", fmt.Sprintf("%s %s", cf.Path, st.dim("("+desc+")")))
	}
	if !d.DockerAvailable {
		return
	}
	if len(d.Running) == 0 {
		line(b, "Running", st.dim("no containers"))
		return
	}
	for i, c := range d.Running {
		label := "Running"
		if i > 0 {
			label = ""
		}
		ports := "no published ports"
		if len(c.Ports) > 0 {
			ports = strings.Join(c.Ports, ", ")
		}
		line(b, label, fmt.Sprintf("%s %s %s %s %s %s %s", c.Name, st.dim(sym.dot), c.Image, st.dim(sym.dot), st.dim(c.Status), st.dim(sym.dot), st.dim(ports)))
	}
}

func writeConfig(b *strings.Builder, st Style, sym symbols, d sections.ConfigInfo) {
	if len(d.EnvFiles) == 0 && len(d.Files) == 0 {
		b.WriteString("  " + st.dim("no configuration files found") + "\n")
		return
	}
	for _, ef := range d.EnvFiles {
		status := ""
		switch {
		case ef.Template:
			status = st.dim("template " + sym.dot + " committed")
		case ef.Gitignore == sections.GitignoreIgnored:
			status = st.green(sym.ok) + " gitignored"
		case ef.Gitignore == sections.GitignoreNotIgnored:
			status = st.yellow(sym.warn) + " " + st.yellow("NOT gitignored")
		case ef.Gitignore == sections.GitignoreTracked:
			status = st.yellow(sym.warn) + " " + st.yellow("tracked by git")
		default:
			status = st.dim("not in a git repository")
		}
		line(b, "Env", fmt.Sprintf("%s %s %s %s %s", ef.Name, st.dim(sym.dot), plural(ef.Variables, "variable"), st.dim(sym.dot), status))
		if len(ef.Keys) > 0 {
			fmt.Fprintf(b, "  %-*s %s\n", labelWidth, "", st.dim("keys: "+strings.Join(ef.Keys, ", ")))
		}
	}
	groups := []struct{ kind, label string }{
		{"config", "Config"}, {"compose", "Compose"}, {"dockerfile", "Dockerfile"},
		{"makefile", "Makefile"}, {"ci", "CI"},
	}
	for _, g := range groups {
		var paths []string
		for _, f := range d.Files {
			if f.Kind == g.kind {
				paths = append(paths, f.Path)
			}
		}
		if len(paths) > 0 {
			line(b, g.label, strings.Join(paths, ", "))
		}
	}
	for _, w := range d.Warnings {
		b.WriteString("  " + st.yellow(sym.warn+" "+w) + "\n")
	}
}
