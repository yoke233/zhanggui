package git

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type FileEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Type      string `json:"type"`
	GitStatus string `json:"git_status"`
}

func (r *Runner) ListDirectory(dirPath string) ([]FileEntry, error) {
	normalizedDir := normalizeRelativePathInput(dirPath, "")

	statusByPath, err := r.statusByPath()
	if err != nil {
		return nil, err
	}

	hasCommit, err := r.hasHeadCommit()
	if err != nil {
		return nil, err
	}
	if !hasCommit {
		return r.listDirectoryFromFS(normalizedDir, statusByPath)
	}
	return r.listDirectoryFromGit(normalizedDir, statusByPath)
}

func (r *Runner) StatusFiles() ([]FileEntry, error) {
	statusByPath, err := r.statusByPath()
	if err != nil {
		return nil, err
	}
	items := make([]FileEntry, 0, len(statusByPath))
	for filePath, status := range statusByPath {
		items = append(items, FileEntry{
			Name:      path.Base(filePath),
			Path:      filePath,
			Type:      "file",
			GitStatus: status,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Path < items[j].Path
	})
	return items, nil
}

func (r *Runner) statusByPath() (map[string]string, error) {
	stdout, stderr, _, err := r.runRaw("status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return nil, fmt.Errorf("git status --porcelain --untracked-files=all: %s: %w", strings.TrimSpace(stderr), err)
	}

	lines := strings.Split(strings.ReplaceAll(stdout, "\r\n", "\n"), "\n")
	statuses := make(map[string]string, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}

		status, filePath, ok := parsePorcelainStatusLine(line)
		if !ok {
			continue
		}
		statuses[filePath] = mergeGitStatus(statuses[filePath], status)
	}
	return statuses, nil
}

func (r *Runner) DiffFile(filePath string) (string, error) {
	normalizedFile := normalizeRelativePathInput(filePath, "")
	if normalizedFile == "" || normalizedFile == "." {
		return "", fmt.Errorf("file path is required")
	}

	diffText, err := r.run("diff", "HEAD", "--", normalizedFile)
	if err == nil && strings.TrimSpace(diffText) != "" {
		return diffText, nil
	}
	if err != nil && isNoHeadError(err.Error()) {
		cachedDiff, cachedErr := r.run("diff", "--cached", "--", normalizedFile)
		if cachedErr == nil {
			if strings.TrimSpace(cachedDiff) != "" {
				return cachedDiff, nil
			}
			diffText = cachedDiff
			err = nil
		} else {
			err = cachedErr
		}
	}

	statusByPath, statusErr := r.statusByPath()
	if statusErr != nil {
		if err != nil {
			return "", err
		}
		return "", statusErr
	}
	if !isUntrackedStatus(statusByPath[normalizedFile]) {
		if err != nil {
			return "", err
		}
		return diffText, nil
	}

	diffText, err = r.diffAgainstNull(normalizedFile)
	if err != nil {
		return "", err
	}
	return diffText, nil
}

func (r *Runner) DiffSummary() (string, error) {
	summary, err := r.run("diff", "--stat", "HEAD")
	if err != nil && !isNoHeadError(err.Error()) {
		return "", err
	}
	summary = strings.TrimSpace(summary)

	statusByPath, statusErr := r.statusByPath()
	if statusErr != nil {
		if err != nil {
			return "", err
		}
		return "", statusErr
	}

	untracked := make([]string, 0, len(statusByPath))
	for filePath, status := range statusByPath {
		if isUntrackedStatus(status) {
			untracked = append(untracked, filePath)
		}
	}
	sort.Strings(untracked)
	if len(untracked) == 0 {
		return summary, nil
	}

	lines := make([]string, 0, len(untracked))
	for _, filePath := range untracked {
		lines = append(lines, "?? "+filePath)
	}
	if summary == "" {
		return strings.Join(lines, "\n"), nil
	}
	return summary + "\n" + strings.Join(lines, "\n"), nil
}

func (r *Runner) listDirectoryFromGit(dirPath string, statusByPath map[string]string) ([]FileEntry, error) {
	pathSpec := dirPath
	if pathSpec == "" {
		pathSpec = "."
	}

	stdout, stderr, _, err := r.runRaw("ls-files", "--cached", "--others", "--exclude-standard", "--full-name", "--", pathSpec)
	if err != nil {
		return nil, fmt.Errorf("git ls-files --cached --others --exclude-standard --full-name -- %s: %s: %w", pathSpec, strings.TrimSpace(stderr), err)
	}

	lines := strings.Split(strings.ReplaceAll(stdout, "\r\n", "\n"), "\n")
	entries := buildEntriesFromPaths(dirPath, lines)
	return finalizeEntries(entries, statusByPath), nil
}

func (r *Runner) listDirectoryFromFS(dirPath string, statusByPath map[string]string) ([]FileEntry, error) {
	targetDir := r.repoDir
	if dirPath != "" && dirPath != "." {
		targetDir = filepath.Join(r.repoDir, filepath.FromSlash(dirPath))
	}

	items, err := os.ReadDir(targetDir)
	if err != nil {
		return nil, err
	}

	entries := make(map[string]FileEntry, len(items))
	for _, item := range items {
		if item.Name() == ".git" {
			continue
		}
		entryPath := item.Name()
		if dirPath != "" && dirPath != "." {
			entryPath = dirPath + "/" + item.Name()
		}
		entryPath = filepath.ToSlash(filepath.Clean(entryPath))
		entryType := "file"
		if item.IsDir() {
			entryType = "dir"
		}
		entries[entryPath] = FileEntry{
			Name: item.Name(),
			Path: entryPath,
			Type: entryType,
		}
	}

	return finalizeEntries(entries, statusByPath), nil
}

func (r *Runner) diffAgainstNull(relativeFilePath string) (string, error) {
	absFilePath := filepath.Join(r.repoDir, filepath.FromSlash(relativeFilePath))
	nullCandidates := []string{os.DevNull}
	if os.DevNull != "/dev/null" {
		nullCandidates = append(nullCandidates, "/dev/null")
	}

	var lastErr error
	for _, nullPath := range nullCandidates {
		stdout, stderr, exitCode, err := r.runRaw("diff", "--no-index", "--", nullPath, absFilePath)
		if err == nil || exitCode == 1 {
			return strings.TrimSpace(stdout), nil
		}
		lastErr = fmt.Errorf("git diff --no-index -- %s %s: %s: %w", nullPath, absFilePath, strings.TrimSpace(stderr), err)
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", nil
}

func (r *Runner) hasHeadCommit() (bool, error) {
	_, stderr, _, err := r.runRaw("rev-parse", "--verify", "HEAD")
	if err != nil {
		if isNoHeadError(stderr) || isNoHeadError(err.Error()) {
			return false, nil
		}
		return false, fmt.Errorf("git rev-parse --verify HEAD: %s: %w", strings.TrimSpace(stderr), err)
	}
	return true, nil
}

func parsePorcelainStatusLine(line string) (string, string, bool) {
	if len(line) < 3 {
		return "", "", false
	}

	status := normalizePorcelainStatus(line[0], line[1])
	pathPart := strings.TrimSpace(line[3:])
	if status == "" || pathPart == "" {
		return "", "", false
	}
	if idx := strings.LastIndex(pathPart, " -> "); idx >= 0 {
		pathPart = strings.TrimSpace(pathPart[idx+4:])
	}
	if unquoted, err := strconv.Unquote(pathPart); err == nil {
		pathPart = unquoted
	}

	normalizedPath := filepath.ToSlash(filepath.Clean(pathPart))
	if normalizedPath == "" || normalizedPath == "." {
		return "", "", false
	}
	return status, normalizedPath, true
}

func buildEntriesFromPaths(baseDir string, filePaths []string) map[string]FileEntry {
	entries := map[string]FileEntry{}
	for _, filePath := range filePaths {
		normalizedPath := filepath.ToSlash(filepath.Clean(strings.TrimSpace(filePath)))
		if normalizedPath == "" || normalizedPath == "." {
			continue
		}

		entryPath, entryName, entryType, ok := extractImmediateEntry(baseDir, normalizedPath)
		if !ok {
			continue
		}

		existing, exists := entries[entryPath]
		if !exists {
			entries[entryPath] = FileEntry{
				Name: entryName,
				Path: entryPath,
				Type: entryType,
			}
			continue
		}
		if existing.Type != "dir" && entryType == "dir" {
			existing.Type = "dir"
			entries[entryPath] = existing
		}
	}
	return entries
}

func extractImmediateEntry(baseDir string, filePath string) (string, string, string, bool) {
	normalizedBase := normalizeRelativePathInput(baseDir, ".")
	relativePath := filePath
	if normalizedBase != "." {
		prefix := strings.TrimSuffix(normalizedBase, "/") + "/"
		if !strings.HasPrefix(filePath, prefix) {
			return "", "", "", false
		}
		relativePath = strings.TrimPrefix(filePath, prefix)
	}
	if relativePath == "" || relativePath == "." {
		return "", "", "", false
	}

	name := relativePath
	entryType := "file"
	if idx := strings.Index(name, "/"); idx >= 0 {
		name = name[:idx]
		entryType = "dir"
	}

	entryPath := name
	if normalizedBase != "." {
		entryPath = strings.TrimSuffix(normalizedBase, "/") + "/" + name
	}
	return entryPath, name, entryType, true
}

func finalizeEntries(entries map[string]FileEntry, statusByPath map[string]string) []FileEntry {
	if len(entries) == 0 {
		return []FileEntry{}
	}

	items := make([]FileEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Type == "dir" {
			entry.GitStatus = statusForDirectory(entry.Path, statusByPath)
		} else {
			entry.GitStatus = statusByPath[entry.Path]
		}
		items = append(items, entry)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Type != items[j].Type {
			return items[i].Type == "dir"
		}
		if items[i].Name == items[j].Name {
			return items[i].Path < items[j].Path
		}
		return items[i].Name < items[j].Name
	})
	return items
}

func statusForDirectory(dirPath string, statusByPath map[string]string) string {
	bestStatus := ""
	bestRank := 0
	prefix := strings.TrimSuffix(dirPath, "/") + "/"
	for filePath, status := range statusByPath {
		if !strings.HasPrefix(filePath, prefix) && filePath != strings.TrimSuffix(prefix, "/") {
			continue
		}
		rank := statusRank(status)
		if rank > bestRank {
			bestRank = rank
			bestStatus = status
		}
	}
	return bestStatus
}

func statusRank(status string) int {
	switch {
	case status == "?":
		return 90
	case status == "M":
		return 70
	case status == "A":
		return 60
	case status == "D":
		return 50
	case status == "R":
		return 40
	case strings.TrimSpace(status) == "":
		return 0
	default:
		return 10
	}
}

func normalizeRelativePathInput(rawPath string, defaultPath string) string {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return defaultPath
	}
	normalized := filepath.ToSlash(filepath.Clean(filepath.FromSlash(trimmed)))
	if normalized == "" || normalized == "." {
		return defaultPath
	}
	return normalized
}

func isNoHeadError(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(lower, "needed a single revision") ||
		strings.Contains(lower, "does not have any commits yet") ||
		strings.Contains(lower, "unknown revision or path not in the working tree") ||
		strings.Contains(lower, "ambiguous argument 'head'") ||
		strings.Contains(lower, "bad revision 'head'")
}

func isUntrackedStatus(status string) bool {
	return strings.TrimSpace(status) == "?"
}

func normalizePorcelainStatus(indexStatus byte, worktreeStatus byte) string {
	switch {
	case indexStatus == '?' && worktreeStatus == '?':
		return "?"
	case indexStatus == 'R' || worktreeStatus == 'R':
		return "R"
	case indexStatus == 'D' || worktreeStatus == 'D':
		return "D"
	case indexStatus == 'A' || worktreeStatus == 'A' || indexStatus == 'C' || worktreeStatus == 'C':
		return "A"
	case indexStatus == 'M' || worktreeStatus == 'M' || indexStatus == 'U' || worktreeStatus == 'U':
		return "M"
	case indexStatus == ' ' && worktreeStatus == ' ':
		return ""
	default:
		return "M"
	}
}

func mergeGitStatus(current string, next string) string {
	if statusRank(next) >= statusRank(current) {
		return next
	}
	return current
}
