package sections

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zahidhasann88/whereami/internal/env"
	"github.com/zahidhasann88/whereami/internal/env/envtest"
	"github.com/zahidhasann88/whereami/internal/system"
)

// Recorded outputs from git 2.47 (porcelain v2).
const (
	statusCleanTracking = "# branch.oid 3f2a9c1d0e4b5a6f7e8d9c0b1a2f3e4d5c6b7a8f\n" +
		"# branch.head main\n" +
		"# branch.upstream origin/main\n" +
		"# branch.ab +0 -0\n"

	statusDirtyAhead = "# branch.oid 3f2a9c1d0e4b5a6f7e8d9c0b1a2f3e4d5c6b7a8f\n" +
		"# branch.head feature/login\n" +
		"# branch.upstream origin/feature/login\n" +
		"# branch.ab +2 -1\n" +
		"1 M. N... 100644 100644 100644 aaa bbb internal/auth/login.go\n" +
		"1 .M N... 100644 100644 100644 ccc ddd README.md\n" +
		"1 MM N... 100644 100644 100644 eee fff cmd/main.go\n" +
		"2 R. N... 100644 100644 100644 ggg hhh R100 old.go\tnew.go\n" +
		"u UU N... 100644 100644 100644 100644 iii jjj kkk conflict.go\n" +
		"? notes.txt\n" +
		"? tmp/\n" +
		"! ignored.log\n"

	statusDetached = "# branch.oid 9d8c7b6a5f4e3d2c1b0a99887766554433221100\n" +
		"# branch.head (detached)\n"

	statusUnborn = "# branch.oid (initial)\n" +
		"# branch.head main\n" +
		"? main.go\n"

	statusNoUpstream = "# branch.oid 1234567890abcdef1234567890abcdef12345678\n" +
		"# branch.head topic\n"
)

func TestParseStatusV2(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want gitStatus
	}{
		{"clean with upstream", statusCleanTracking, gitStatus{
			oid: "3f2a9c1d0e4b5a6f7e8d9c0b1a2f3e4d5c6b7a8f", head: "main", upstream: "origin/main", hasAB: true,
		}},
		{"dirty and ahead", statusDirtyAhead, gitStatus{
			oid: "3f2a9c1d0e4b5a6f7e8d9c0b1a2f3e4d5c6b7a8f", head: "feature/login", upstream: "origin/feature/login",
			ahead: 2, behind: 1, hasAB: true,
			staged: 3, modified: 2, untracked: 2, conflicted: 1,
		}},
		{"detached", statusDetached, gitStatus{oid: "9d8c7b6a5f4e3d2c1b0a99887766554433221100", head: "(detached)"}},
		{"unborn", statusUnborn, gitStatus{oid: "(initial)", head: "main", untracked: 1}},
		{"no upstream", statusNoUpstream, gitStatus{oid: "1234567890abcdef1234567890abcdef12345678", head: "topic"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseStatusV2(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("got %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestParseStatusV2RejectsGarbage(t *testing.T) {
	if _, err := ParseStatusV2("# branch.ab +x -y\n"); err == nil {
		t.Error("expected error for malformed branch.ab")
	}
}

func TestParseLastCommit(t *testing.T) {
	c, err := ParseLastCommit("a1b2c3d\x1fAdd login handler\x1f2 hours ago\x1fJane Doe\x1f2026-10-10T08:00:00+06:00\n")
	if err != nil {
		t.Fatal(err)
	}
	if c.Hash != "a1b2c3d" || c.Subject != "Add login handler" || c.RelativeTime != "2 hours ago" || c.Author != "Jane Doe" {
		t.Errorf("unexpected commit %+v", c)
	}
	if c.Date != "2026-10-10T08:00:00+06:00" {
		t.Errorf("date = %q", c.Date)
	}
	if _, err := ParseLastCommit("only\x1ftwo"); err == nil {
		t.Error("expected error for wrong field count")
	}
	if c, err := ParseLastCommit(""); c != nil || err != nil {
		t.Errorf("empty log should yield nil commit, got %v %v", c, err)
	}
}

func TestCountStashes(t *testing.T) {
	if n := CountStashes(""); n != 0 {
		t.Errorf("got %d", n)
	}
	if n := CountStashes("stash@{0}: WIP on main\nstash@{1}: On dev: save\n"); n != 2 {
		t.Errorf("got %d", n)
	}
}

func TestStripCredentials(t *testing.T) {
	cases := map[string]string{
		"https://github.com/acme/api.git":            "https://github.com/acme/api.git",
		"https://ghp_secrettoken@github.com/a/b.git": "https://github.com/a/b.git",
		"https://user:p%40ss@example.com/a/b.git":    "https://example.com/a/b.git",
		"ssh://git:pw@host.example:2222/x.git":       "ssh://host.example:2222/x.git",
		"git@github.com:acme/api.git":                "git@github.com:acme/api.git",
		"/srv/git/repo.git":                          "/srv/git/repo.git",
	}
	for in, want := range cases {
		if got := StripCredentials(in); got != want {
			t.Errorf("StripCredentials(%q) = %q, want %q", in, got, want)
		}
		if strings.Contains(StripCredentials(in), "secret") || strings.Contains(StripCredentials(in), "p%40ss") {
			t.Errorf("credentials leaked for %q", in)
		}
	}
}

func fakeGitEnv(t *testing.T, replies map[string]envtest.Reply) *env.Env {
	t.Helper()
	return &env.Env{Dir: t.TempDir(), Runner: envtest.NewRunner(replies), FS: system.OSFS{}}
}

func TestGitCollectRecordedDirtyRepo(t *testing.T) {
	replies := map[string]envtest.Reply{
		"git rev-parse --is-inside-work-tree": {Stdout: "true\n"},
		"git status --porcelain=v2 --branch":  {Stdout: statusDirtyAhead},
		"git log -1 --format=%h%x1f%s%x1f%cr%x1f%an%x1f%cI": {
			Stdout: "a1b2c3d\x1fAdd login handler\x1f2 hours ago\x1fJane Doe\x1f2026-10-10T08:00:00+06:00\n",
		},
		"git remote":                {Stdout: "upstream\norigin\n"},
		"git remote get-url origin": {Stdout: "https://ghp_TOKEN@github.com/acme/api.git\n"},
		"git stash list":            {Stdout: "stash@{0}: WIP\n"},
	}
	res, err := Git{}.Collect(context.Background(), fakeGitEnv(t, replies))
	if err != nil {
		t.Fatal(err)
	}
	info := res.Data.(GitInfo)
	if info.Branch != "feature/login" || info.Upstream != "origin/feature/login" || info.Ahead != 2 || info.Behind != 1 {
		t.Errorf("branch info %+v", info)
	}
	if info.Clean || info.Staged != 3 || info.Modified != 2 || info.Untracked != 2 || info.Conflicted != 1 {
		t.Errorf("counts %+v", info)
	}
	if info.LastCommit == nil || info.LastCommit.Hash != "a1b2c3d" {
		t.Errorf("last commit %+v", info.LastCommit)
	}
	if info.Remote == nil || info.Remote.Name != "origin" || info.Remote.URL != "https://github.com/acme/api.git" {
		t.Errorf("remote %+v", info.Remote)
	}
	if info.Stashes != 1 {
		t.Errorf("stashes = %d", info.Stashes)
	}
}

func TestGitCollectUnavailableReasons(t *testing.T) {
	cases := []struct {
		name    string
		replies map[string]envtest.Reply
		reason  string
	}{
		{"not a repo", map[string]envtest.Reply{
			"git rev-parse --is-inside-work-tree": {Err: errors.New("exit status 128"), Stderr: "fatal: not a git repository"},
		}, "not a git repository"},
		{"git missing", map[string]envtest.Reply{}, "git is not installed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Git{}.Collect(context.Background(), fakeGitEnv(t, tc.replies))
			if err != nil {
				t.Fatal(err)
			}
			if res.Status != StatusUnavailable || res.Reason != tc.reason {
				t.Errorf("got %s / %q, want unavailable / %q", res.Status, res.Reason, tc.reason)
			}
		})
	}
}

func TestGitCollectDetachedAndUnborn(t *testing.T) {
	detached := map[string]envtest.Reply{
		"git rev-parse --is-inside-work-tree": {Stdout: "true\n"},
		"git status --porcelain=v2 --branch":  {Stdout: statusDetached},
		"git stash list":                      {},
	}
	res, _ := Git{}.Collect(context.Background(), fakeGitEnv(t, detached))
	info := res.Data.(GitInfo)
	if !info.Detached || info.Branch != "" || info.Head != "9d8c7b6" || info.Remote != nil {
		t.Errorf("detached info %+v", info)
	}

	unborn := map[string]envtest.Reply{
		"git rev-parse --is-inside-work-tree": {Stdout: "true\n"},
		"git status --porcelain=v2 --branch":  {Stdout: statusUnborn},
		"git stash list":                      {},
	}
	res, _ = Git{}.Collect(context.Background(), fakeGitEnv(t, unborn))
	info = res.Data.(GitInfo)
	if !info.Unborn || info.Branch != "main" || info.LastCommit != nil || info.Untracked != 1 {
		t.Errorf("unborn info %+v", info)
	}
}

// requireGit skips integration tests when git is not installed.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func realEnv(dir string) *env.Env {
	return &env.Env{Dir: dir, Runner: system.OSRunner{}, FS: system.OSFS{}}
}

func TestGitIntegrationLifecycle(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	gitRun(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")

	// No commits yet.
	res, err := Git{}.Collect(context.Background(), realEnv(dir))
	if err != nil {
		t.Fatal(err)
	}
	info := res.Data.(GitInfo)
	if !info.Unborn || info.Branch != "main" || info.LastCommit != nil {
		t.Errorf("unborn: %+v", info)
	}
	if info.Remote != nil {
		t.Errorf("unexpected remote %+v", info.Remote)
	}

	// First commit, then dirty the tree in every way we report.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "a.txt")
	gitRun(t, dir, "commit", "-q", "-m", "Initial commit")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "b.txt")
	if err := os.WriteFile(filepath.Join(dir, "c.txt"), []byte("untracked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, _ = Git{}.Collect(context.Background(), realEnv(dir))
	info = res.Data.(GitInfo)
	if info.Unborn || info.Clean {
		t.Fatalf("expected dirty repo: %+v", info)
	}
	if info.Staged != 1 || info.Modified != 1 || info.Untracked != 1 {
		t.Errorf("counts staged=%d modified=%d untracked=%d", info.Staged, info.Modified, info.Untracked)
	}
	if info.LastCommit == nil || info.LastCommit.Subject != "Initial commit" || info.LastCommit.Author != "Test" {
		t.Errorf("last commit %+v", info.LastCommit)
	}
	if info.Upstream != "" {
		t.Errorf("upstream = %q, want none", info.Upstream)
	}

	// Stash the work and add a remote with embedded credentials.
	gitRun(t, dir, "stash", "push", "-q", "-u", "-m", "wip")
	gitRun(t, dir, "remote", "add", "origin", "https://ghp_SUPERSECRET@example.com/acme/api.git")
	res, _ = Git{}.Collect(context.Background(), realEnv(dir))
	info = res.Data.(GitInfo)
	if info.Stashes != 1 {
		t.Errorf("stashes = %d", info.Stashes)
	}
	if info.Remote == nil || info.Remote.URL != "https://example.com/acme/api.git" {
		t.Errorf("remote = %+v", info.Remote)
	}
	if !info.Clean {
		t.Errorf("expected clean after stash: %+v", info)
	}
}

func TestGitIntegrationDetachedHead(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "a"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "a")
	gitRun(t, dir, "commit", "-q", "-m", "one")
	gitRun(t, dir, "checkout", "-q", "--detach")
	res, _ := Git{}.Collect(context.Background(), realEnv(dir))
	info := res.Data.(GitInfo)
	if !info.Detached || len(info.Head) != 7 {
		t.Errorf("detached info %+v", info)
	}
}

func TestGitIntegrationNotARepo(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	res, _ := Git{}.Collect(context.Background(), realEnv(dir))
	// Skip this check if the temporary directory is inside a repository.
	if res.Status == StatusOK {
		t.Skip("temp directory is inside a git repository on this machine")
	}
	if res.Status != StatusUnavailable || res.Reason != "not a git repository" {
		t.Errorf("got %s / %q", res.Status, res.Reason)
	}
}
