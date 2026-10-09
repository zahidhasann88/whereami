package sections

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"

	"github.com/zahidhasann88/whereami/internal/env"
)

// Git reports branch, upstream, working tree state, last commit, remote and stashes.
type Git struct{}

func (Git) Name() string { return "git" }

// GitInfo is the data of the git section.
type GitInfo struct {
	Branch     string      `json:"branch,omitempty"`
	Detached   bool        `json:"detached"`
	Unborn     bool        `json:"unborn"`
	Head       string      `json:"head,omitempty"`
	Upstream   string      `json:"upstream,omitempty"`
	Ahead      int         `json:"ahead"`
	Behind     int         `json:"behind"`
	Clean      bool        `json:"clean"`
	Staged     int         `json:"staged"`
	Modified   int         `json:"modified"`
	Untracked  int         `json:"untracked"`
	Conflicted int         `json:"conflicted"`
	LastCommit *CommitInfo `json:"last_commit,omitempty"`
	Remote     *RemoteInfo `json:"remote,omitempty"`
	Stashes    int         `json:"stashes"`
}

// CommitInfo describes the most recent commit.
type CommitInfo struct {
	Hash         string `json:"hash"`
	Subject      string `json:"subject"`
	RelativeTime string `json:"relative_time"`
	Date         string `json:"date"`
	Author       string `json:"author"`
}

// RemoteInfo describes the remote used for display.
type RemoteInfo struct {
	Name string `json:"name"`
	URL  string `json:"url"` // credentials removed
}

// gitStatus is the parsed output of `git status --porcelain=v2 --branch`.
type gitStatus struct {
	oid        string
	head       string
	upstream   string
	ahead      int
	behind     int
	hasAB      bool
	staged     int
	modified   int
	untracked  int
	conflicted int
}

// ParseStatusV2 parses `git status --porcelain=v2 --branch` output.
func ParseStatusV2(out string) (gitStatus, error) {
	var st gitStatus
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "# branch.oid "):
			st.oid = strings.TrimPrefix(line, "# branch.oid ")
		case strings.HasPrefix(line, "# branch.head "):
			st.head = strings.TrimPrefix(line, "# branch.head ")
		case strings.HasPrefix(line, "# branch.upstream "):
			st.upstream = strings.TrimPrefix(line, "# branch.upstream ")
		case strings.HasPrefix(line, "# branch.ab "):
			fields := strings.Fields(strings.TrimPrefix(line, "# branch.ab "))
			if len(fields) != 2 {
				return st, fmt.Errorf("unexpected branch.ab line: %q", line)
			}
			a, errA := strconv.Atoi(strings.TrimPrefix(fields[0], "+"))
			b, errB := strconv.Atoi(strings.TrimPrefix(fields[1], "-"))
			if errA != nil || errB != nil {
				return st, fmt.Errorf("unexpected branch.ab line: %q", line)
			}
			st.ahead, st.behind, st.hasAB = a, b, true
		case strings.HasPrefix(line, "1 "), strings.HasPrefix(line, "2 "):
			fields := strings.Fields(line)
			if len(fields) < 2 || len(fields[1]) != 2 {
				return st, fmt.Errorf("unexpected entry: %q", line)
			}
			x, y := fields[1][0], fields[1][1]
			if x != '.' {
				st.staged++
			}
			if y != '.' {
				st.modified++
			}
		case strings.HasPrefix(line, "u "):
			st.conflicted++
		case strings.HasPrefix(line, "? "):
			st.untracked++
		}
	}
	return st, nil
}

// ParseLastCommit reads five unit-separator-delimited Git log fields.
func ParseLastCommit(out string) (*CommitInfo, error) {
	rec := strings.TrimRight(out, "\r\n")
	if rec == "" {
		return nil, nil
	}
	parts := strings.Split(rec, "\x1f")
	if len(parts) != 5 {
		return nil, fmt.Errorf("unexpected log format: %d fields", len(parts))
	}
	return &CommitInfo{
		Hash:         parts[0],
		Subject:      parts[1],
		RelativeTime: parts[2],
		Author:       parts[3],
		Date:         parts[4],
	}, nil
}

// CountStashes counts entries in `git stash list` output.
func CountStashes(out string) int {
	n := 0
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}

// StripCredentials removes URL user information, leaving SCP-style remotes unchanged.
func StripCredentials(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.User == nil {
		return raw
	}
	u.User = nil
	return u.String()
}

func (Git) Collect(ctx context.Context, e *env.Env) (Result, error) {
	run := func(args ...string) (string, error) {
		out, err := e.Runner.Run(ctx, e.Dir, "git", args...)
		return strings.TrimSpace(string(out.Stdout)), err
	}

	inside, err := run("rev-parse", "--is-inside-work-tree")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return unavailable("git is not installed"), nil
		}
		return unavailable("not a git repository"), nil
	}
	if inside != "true" {
		return unavailable("not a git working tree"), nil
	}

	raw, err := run("status", "--porcelain=v2", "--branch")
	if err != nil {
		return unavailable(fmt.Sprintf("git status failed: %s", firstLine(err))), nil
	}
	st, err := ParseStatusV2(raw)
	if err != nil {
		return unavailable(fmt.Sprintf("could not parse git status: %v", err)), nil
	}

	info := &GitInfo{
		Detached:   st.head == "(detached)",
		Unborn:     st.oid == "(initial)",
		Upstream:   st.upstream,
		Ahead:      st.ahead,
		Behind:     st.behind,
		Staged:     st.staged,
		Modified:   st.modified,
		Untracked:  st.untracked,
		Conflicted: st.conflicted,
	}
	info.Clean = st.staged == 0 && st.modified == 0 && st.untracked == 0 && st.conflicted == 0
	if info.Detached {
		if !info.Unborn {
			info.Head = shortHash(st.oid)
		}
	} else {
		info.Branch = st.head
	}
	if !info.Unborn && st.oid != "" {
		info.Head = shortHash(st.oid)
	}

	if !info.Unborn {
		logOut, err := run("log", "-1", "--format=%h%x1f%s%x1f%cr%x1f%an%x1f%cI")
		if err == nil {
			if c, perr := ParseLastCommit(logOut); perr == nil {
				info.LastCommit = c
			}
		}
	}

	if remote, ok := pickRemote(ctx, e); ok {
		info.Remote = remote
	}

	if stashOut, err := run("stash", "list"); err == nil {
		info.Stashes = CountStashes(stashOut)
	}

	return Result{Status: StatusOK, Data: *info}, nil
}

// pickRemote returns "origin" if present, otherwise the first configured remote.
func pickRemote(ctx context.Context, e *env.Env) (*RemoteInfo, bool) {
	out, err := e.Runner.Run(ctx, e.Dir, "git", "remote")
	if err != nil {
		return nil, false
	}
	var names []string
	for _, n := range strings.Split(strings.TrimSpace(string(out.Stdout)), "\n") {
		if n = strings.TrimSpace(n); n != "" {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return nil, false
	}
	name := names[0]
	for _, n := range names {
		if n == "origin" {
			name = n
		}
	}
	u, err := e.Runner.Run(ctx, e.Dir, "git", "remote", "get-url", name)
	if err != nil {
		return nil, false
	}
	return &RemoteInfo{Name: name, URL: StripCredentials(strings.TrimSpace(string(u.Stdout)))}, true
}

func unavailable(reason string) Result {
	return Result{Status: StatusUnavailable, Reason: reason}
}

func shortHash(oid string) string {
	if len(oid) > 7 {
		return oid[:7]
	}
	return oid
}

func firstLine(err error) string {
	msg := err.Error()
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	return msg
}
