// vendor-sync clones/updates a GitHub repository and copies its docs into docs/vendor/.
//
// Usage:
//
//	go run scripts/vendor-sync/main.go -repo volcengine/OpenViking
//	go run scripts/vendor-sync/main.go -preset openviking
//	go run scripts/vendor-sync/main.go -preset a2a -depth1
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// preset defines default parameters for well-known repositories.
type preset struct {
	Repo    string
	DocsSrc string
	DocsDst string
}

var presets = map[string]preset{
	"openviking": {
		Repo:    "volcengine/OpenViking",
		DocsSrc: "docs",
		DocsDst: "docs/vendor/openviking-upstream-docs",
	},
	"a2a": {
		Repo:    "a2aproject/A2A",
		DocsSrc: "docs",
		DocsDst: "docs/vendor/a2a-protocol-upstream-docs",
	},
	"acp": {
		Repo:    "agentclientprotocol/agent-client-protocol",
		DocsSrc: "docs",
		DocsDst: "docs/vendor/acp-upstream-docs",
	},
}

type repoSpec struct {
	Host      string
	Owner     string
	Repo      string
	GitRemote string
}

func (r repoSpec) Display() string {
	return fmt.Sprintf("%s/%s/%s", r.Host, r.Owner, r.Repo)
}

func (r repoSpec) PathToken() string {
	raw := fmt.Sprintf("%s-%s-%s", r.Host, r.Owner, r.Repo)
	re := regexp.MustCompile(`[^A-Za-z0-9._-]`)
	return re.ReplaceAllString(raw, "-")
}

type refInfo struct {
	Kind string // "branch", "tag", "commit"
	Name string
}

func main() {
	var (
		repoFlag   string
		presetFlag string
		refFlag    string
		docsSrc    string
		docsDst    string
		targetPath string
		force      bool
		depth1     bool
	)

	flag.StringVar(&repoFlag, "repo", "", "GitHub repository (owner/repo or full URL)")
	flag.StringVar(&presetFlag, "preset", "", "Use preset config: openviking, a2a, acp")
	flag.StringVar(&refFlag, "ref", "", "Branch, tag, or commit (default: remote default branch)")
	flag.StringVar(&docsSrc, "docs-src", "docs", "Docs source path inside the repo")
	flag.StringVar(&docsDst, "docs-dst", "", "Docs destination path (relative to project root)")
	flag.StringVar(&targetPath, "target", "", "Clone target directory (default: .tmp/github-repos/<token>)")
	flag.BoolVar(&force, "force", false, "Force reset on dirty tree or origin mismatch")
	flag.BoolVar(&depth1, "depth1", false, "Shallow clone (depth=1)")
	flag.Parse()

	// Apply preset if specified.
	if presetFlag != "" {
		p, ok := presets[strings.ToLower(presetFlag)]
		if !ok {
			var names []string
			for k := range presets {
				names = append(names, k)
			}
			fatalf("unknown preset %q, available: %s", presetFlag, strings.Join(names, ", "))
		}
		if repoFlag == "" {
			repoFlag = p.Repo
		}
		if docsSrc == "docs" {
			docsSrc = p.DocsSrc
		}
		if docsDst == "" {
			docsDst = p.DocsDst
		}
	}

	if repoFlag == "" {
		fatalf("must specify -repo or -preset")
	}

	// Resolve project root (scripts/vendor-sync/../../).
	exePath, err := os.Getwd()
	if err != nil {
		fatalf("getwd: %v", err)
	}
	baseDir := findProjectRoot(exePath)

	spec, err := resolveRepo(repoFlag)
	if err != nil {
		fatalf("resolve repo: %v", err)
	}

	if targetPath == "" {
		targetPath = filepath.Join(".tmp", "github-repos", spec.PathToken())
	}
	targetAbs := absPath(targetPath, baseDir)

	if docsDst == "" {
		docsDst = filepath.Join("docs", "vendor", spec.PathToken()+"-upstream-docs")
	}

	ref, err := resolveRef(spec.GitRemote, refFlag)
	if err != nil {
		fatalf("resolve ref: %v", err)
	}

	logf("project root: %s", baseDir)
	logf("repository:   %s", spec.Display())
	logf("git remote:   %s", spec.GitRemote)
	logf("target dir:   %s", targetAbs)
	logf("ref:          %s %s", ref.Kind, ref.Name)

	// Clone or fetch.
	if _, err := os.Stat(targetAbs); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(targetAbs), 0o755); err != nil {
			fatalf("mkdir: %v", err)
		}
		cloneArgs := []string{"clone"}
		if depth1 && ref.Kind != "commit" {
			cloneArgs = append(cloneArgs, "--depth=1")
		}
		if ref.Kind == "branch" || ref.Kind == "tag" {
			cloneArgs = append(cloneArgs, "--branch", ref.Name)
		}
		if ref.Kind == "branch" {
			cloneArgs = append(cloneArgs, "--single-branch")
		}
		cloneArgs = append(cloneArgs, spec.GitRemote, targetAbs)
		logf("cloning...")
		git(cloneArgs...)
	} else {
		gitDir := filepath.Join(targetAbs, ".git")
		if _, err := os.Stat(gitDir); os.IsNotExist(err) {
			fatalf("target exists but is not a git repo: %s", targetAbs)
		}

		// Check origin matches.
		currentOrigin := gitC(targetAbs, "remote", "get-url", "origin")
		if !originsMatch(currentOrigin, spec) {
			if !force {
				fatalf("origin mismatch: current=%s, expected=%s (use -force to override)", currentOrigin, spec.GitRemote)
			}
			logf("origin mismatch, updating to %s", spec.GitRemote)
			gitC(targetAbs, "remote", "set-url", "origin", spec.GitRemote)
		}

		if force {
			status := gitC(targetAbs, "status", "--porcelain")
			if status != "" {
				logf("dirty tree, force cleaning...")
				gitC(targetAbs, "reset", "--hard")
				gitC(targetAbs, "clean", "-fd")
			}
		}

		logf("fetching...")
		gitC(targetAbs, "fetch", "--tags", "--prune", "origin")
	}

	// Checkout.
	switch ref.Kind {
	case "branch":
		// Check if local branch exists.
		cmd := exec.Command("git", "-C", targetAbs, "show-ref", "--verify", "--quiet", "refs/heads/"+ref.Name)
		if cmd.Run() == nil {
			gitC(targetAbs, "checkout", ref.Name)
		} else {
			gitC(targetAbs, "checkout", "-b", ref.Name, "--track", "origin/"+ref.Name)
		}
		if force {
			gitC(targetAbs, "reset", "--hard", "origin/"+ref.Name)
			gitC(targetAbs, "clean", "-fd")
		} else {
			gitC(targetAbs, "pull", "--ff-only", "origin", ref.Name)
		}
	case "tag":
		gitC(targetAbs, "checkout", "--detach", "refs/tags/"+ref.Name)
	case "commit":
		cmd := exec.Command("git", "-C", targetAbs, "cat-file", "-e", ref.Name+"^{commit}")
		if cmd.Run() != nil {
			gitC(targetAbs, "fetch", "origin", ref.Name)
		}
		gitC(targetAbs, "checkout", "--detach", ref.Name)
	}

	head := gitC(targetAbs, "rev-parse", "--short", "HEAD")
	headDate := gitC(targetAbs, "show", "-s", "--format=%ci", "HEAD")
	logf("HEAD: %s (%s)", head, headDate)

	// Copy docs.
	docsSrcAbs := filepath.Join(targetAbs, docsSrc)
	docsDstAbs := absPath(docsDst, baseDir)

	if _, err := os.Stat(docsSrcAbs); os.IsNotExist(err) {
		fatalf("docs source not found: %s (use -docs-src to override)", docsSrcAbs)
	}

	if err := os.MkdirAll(docsDstAbs, 0o755); err != nil {
		fatalf("mkdir docs dst: %v", err)
	}

	if err := copyDir(docsSrcAbs, docsDstAbs); err != nil {
		fatalf("copy docs: %v", err)
	}
	logf("docs synced: %s -> %s", docsSrcAbs, docsDstAbs)
	logf("done")
}

// resolveRepo parses various GitHub repository formats into a repoSpec.
func resolveRepo(raw string) (repoSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return repoSpec{}, fmt.Errorf("repository must not be empty")
	}

	// owner/repo (short format).
	if m := regexp.MustCompile(`^([^/\s]+)/([^/\s]+)$`).FindStringSubmatch(raw); m != nil {
		owner, repo := m[1], trimGitSuffix(m[2])
		return repoSpec{
			Host:      "github.com",
			Owner:     owner,
			Repo:      repo,
			GitRemote: fmt.Sprintf("https://github.com/%s/%s.git", owner, repo),
		}, nil
	}

	// host/owner/repo (no scheme).
	if m := regexp.MustCompile(`^([^/\s]+\.[^/\s]+)/([^/\s]+)/([^/\s]+)$`).FindStringSubmatch(raw); m != nil {
		host, owner, repo := m[1], m[2], trimGitSuffix(m[3])
		return repoSpec{
			Host:      host,
			Owner:     owner,
			Repo:      repo,
			GitRemote: fmt.Sprintf("https://%s/%s/%s.git", host, owner, repo),
		}, nil
	}

	// git@host:owner/repo.git (SSH).
	if m := regexp.MustCompile(`^([^@/\s]+)@([^:/\s]+):([^/\s]+)/([^/\s]+?)(?:\.git)?/?$`).FindStringSubmatch(raw); m != nil {
		user, host, owner, repo := m[1], m[2], m[3], trimGitSuffix(m[4])
		return repoSpec{
			Host:      host,
			Owner:     owner,
			Repo:      repo,
			GitRemote: fmt.Sprintf("%s@%s:%s/%s.git", user, host, owner, repo),
		}, nil
	}

	// URL format (https://, ssh://).
	u, err := url.Parse(raw)
	if err == nil && u.Host != "" {
		segments := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(segments) != 2 {
			return repoSpec{}, fmt.Errorf("URL must be host/owner/repo: %s", raw)
		}
		owner, repo := segments[0], trimGitSuffix(segments[1])
		return repoSpec{
			Host:      u.Host,
			Owner:     owner,
			Repo:      repo,
			GitRemote: strings.TrimRight(raw, "/"),
		}, nil
	}

	return repoSpec{}, fmt.Errorf("unsupported repository format: %s", raw)
}

// resolveRef determines whether a ref is a branch, tag, or commit hash.
func resolveRef(remote, ref string) (refInfo, error) {
	if ref == "" {
		branch, err := remoteDefaultBranch(remote)
		if err != nil {
			return refInfo{}, err
		}
		return refInfo{Kind: "branch", Name: branch}, nil
	}

	// SHA-like.
	if regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`).MatchString(ref) {
		return refInfo{Kind: "commit", Name: ref}, nil
	}

	// Check branch.
	out, err := gitOutput("ls-remote", "--heads", remote, "refs/heads/"+ref)
	if err == nil && strings.TrimSpace(out) != "" {
		return refInfo{Kind: "branch", Name: ref}, nil
	}

	// Check tag.
	out, err = gitOutput("ls-remote", "--tags", remote, "refs/tags/"+ref)
	if err == nil && strings.TrimSpace(out) != "" {
		return refInfo{Kind: "tag", Name: ref}, nil
	}

	return refInfo{}, fmt.Errorf("ref not found on remote: %s", ref)
}

func remoteDefaultBranch(remote string) (string, error) {
	out, err := gitOutput("ls-remote", "--symref", remote, "HEAD")
	if err != nil {
		return "", fmt.Errorf("ls-remote failed: %w", err)
	}
	for _, line := range strings.Split(out, "\n") {
		if m := regexp.MustCompile(`^ref:\s+refs/heads/(\S+)\s+HEAD$`).FindStringSubmatch(line); m != nil {
			return m[1], nil
		}
	}
	return "main", nil
}

func originsMatch(current string, spec repoSpec) bool {
	parsed, err := resolveRepo(strings.TrimSpace(current))
	if err != nil {
		return strings.TrimRight(current, "/") == strings.TrimRight(spec.GitRemote, "/")
	}
	return strings.EqualFold(parsed.Host, spec.Host) &&
		strings.EqualFold(parsed.Owner, spec.Owner) &&
		strings.EqualFold(parsed.Repo, spec.Repo)
}

func trimGitSuffix(s string) string {
	return strings.TrimSuffix(s, ".git")
}

// git helpers

func git(args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatalf("git %s: %v", strings.Join(args, " "), err)
	}
}

func gitC(dir string, args ...string) string {
	full := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fatalf("git -C %s %s: %v\n%s", dir, strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// copyDir recursively copies src to dst, overwriting existing files.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func absPath(p, base string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

// findProjectRoot walks up from dir looking for go.mod.
func findProjectRoot(dir string) string {
	d := dir
	for {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return dir // fallback to cwd
		}
		d = parent
	}
}

func logf(format string, args ...any) {
	fmt.Printf("[vendor-sync] "+format+"\n", args...)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[vendor-sync] ERROR: "+format+"\n", args...)
	os.Exit(1)
}
