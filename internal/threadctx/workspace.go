package threadctx

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

const (
	contextFileName = ".context.json"
)

type Store interface {
	GetThread(ctx context.Context, id int64) (*core.Thread, error)
	GetProject(ctx context.Context, id int64) (*core.Project, error)
	ListThreadMembers(ctx context.Context, threadID int64) ([]*core.ThreadMember, error)
	ListThreadContextRefs(ctx context.Context, threadID int64) ([]*core.ThreadContextRef, error)
	ListThreadAttachments(ctx context.Context, threadID int64) ([]*core.ThreadAttachment, error)
	ListResourceSpaces(ctx context.Context, projectID int64) ([]*core.ResourceSpace, error)
}

type PathsInfo struct {
	ThreadDir      string
	ProjectsDir    string
	AttachmentsDir string
	ContextFile    string
}

type ResolvedMount struct {
	Slug          string
	Project       *core.Project
	TargetPath    string
	Access        core.ContextAccess
	CheckCommands []string
}

var slugSanitizer = regexp.MustCompile(`[^a-z0-9-]`)

func Paths(dataDir string, threadID int64) PathsInfo {
	threadDir := filepath.Join(strings.TrimSpace(dataDir), "threads", strconv.FormatInt(threadID, 10))
	return PathsInfo{
		ThreadDir:      threadDir,
		ProjectsDir:    filepath.Join(threadDir, "projects"),
		AttachmentsDir: filepath.Join(threadDir, "attachments"),
		ContextFile:    filepath.Join(threadDir, contextFileName),
	}
}

func EnsureLayout(dataDir string, threadID int64) (PathsInfo, error) {
	if strings.TrimSpace(dataDir) == "" {
		return PathsInfo{}, nil
	}
	paths := Paths(dataDir, threadID)
	for _, dir := range []string{paths.ThreadDir, paths.ProjectsDir, paths.AttachmentsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return PathsInfo{}, fmt.Errorf("create thread workspace dir %q: %w", dir, err)
		}
	}
	return paths, nil
}

func LoadContextFile(dataDir string, threadID int64) (*core.ThreadWorkspaceContext, error) {
	paths := Paths(dataDir, threadID)
	raw, err := os.ReadFile(paths.ContextFile)
	if err != nil {
		return nil, err
	}
	var payload core.ThreadWorkspaceContext
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode thread context: %w", err)
	}
	return &payload, nil
}

func SyncContextFile(ctx context.Context, store Store, dataDir string, threadID int64) (*core.ThreadWorkspaceContext, error) {
	if strings.TrimSpace(dataDir) == "" {
		return nil, nil
	}
	if store == nil {
		return nil, fmt.Errorf("thread context store is nil")
	}

	paths, err := EnsureLayout(dataDir, threadID)
	if err != nil {
		return nil, err
	}

	payload, err := BuildWorkspaceContext(ctx, store, dataDir, threadID)
	if err != nil {
		return nil, err
	}
	if err := syncMountAliasDirs(paths.ProjectsDir, payload, resolvedMountTargets(ctx, store, threadID)); err != nil {
		return nil, err
	}

	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode thread context: %w", err)
	}
	if err := os.WriteFile(paths.ContextFile, append(b, '\n'), 0o644); err != nil {
		return nil, fmt.Errorf("write thread context file: %w", err)
	}
	return payload, nil
}

func BuildWorkspaceContext(ctx context.Context, store Store, dataDir string, threadID int64) (*core.ThreadWorkspaceContext, error) {
	if store == nil {
		return nil, fmt.Errorf("thread context store is nil")
	}

	if _, err := store.GetThread(ctx, threadID); err != nil {
		return nil, err
	}

	refs, err := store.ListThreadContextRefs(ctx, threadID)
	if err != nil {
		return nil, err
	}
	members, err := store.ListThreadMembers(ctx, threadID)
	if err != nil {
		return nil, err
	}

	payload := &core.ThreadWorkspaceContext{
		ThreadID:  threadID,
		Workspace: ".",
		Mounts:    map[string]core.ThreadWorkspaceMount{},
		UpdatedAt: nowUTC(),
	}

	memberSet := make(map[string]struct{})
	for _, member := range members {
		if member == nil {
			continue
		}
		id := strings.TrimSpace(member.UserID)
		if id == "" {
			continue
		}
		memberSet[id] = struct{}{}
	}
	payload.Members = make([]string, 0, len(memberSet))
	for id := range memberSet {
		payload.Members = append(payload.Members, id)
	}
	sort.Strings(payload.Members)

	for _, ref := range refs {
		mount, err := ResolveMount(ctx, store, ref)
		if err != nil || mount == nil {
			continue
		}
		payload.Mounts[mount.Slug] = core.ThreadWorkspaceMount{
			Path:          filepath.ToSlash(filepath.Join("projects", mount.Slug)),
			ProjectID:     mount.Project.ID,
			Access:        mount.Access,
			CheckCommands: append([]string(nil), mount.CheckCommands...),
		}
	}

	if len(payload.Mounts) == 0 {
		payload.Mounts = nil
	}

	// Populate attachments.
	attachments, err := store.ListThreadAttachments(ctx, threadID)
	if err == nil && len(attachments) > 0 {
		for _, att := range attachments {
			if att == nil {
				continue
			}
			payload.Attachments = append(payload.Attachments, core.ThreadWorkspaceAttachmentRef{
				FileName:    att.FileName,
				FilePath:    filepath.ToSlash(filepath.Join("attachments", filepath.Base(att.FilePath))),
				IsDirectory: att.IsDirectory,
				Note:        att.Note,
			})
		}
	}

	return payload, nil
}

func ResolveMount(ctx context.Context, store Store, ref *core.ThreadContextRef) (*ResolvedMount, error) {
	if store == nil {
		return nil, fmt.Errorf("thread context store is nil")
	}
	if ref == nil {
		return nil, fmt.Errorf("thread context ref is nil")
	}
	project, err := store.GetProject(ctx, ref.ProjectID)
	if err != nil {
		return nil, err
	}
	spaces, err := store.ListResourceSpaces(ctx, ref.ProjectID)
	if err != nil {
		return nil, err
	}

	targetPath, checkCommands := resolveSpaceTarget(spaces)
	if targetPath == "" {
		return nil, fmt.Errorf("project %d has no resolvable workspace space", ref.ProjectID)
	}
	return &ResolvedMount{
		Slug:          projectSlug(project),
		Project:       project,
		TargetPath:    targetPath,
		Access:        ref.Access,
		CheckCommands: checkCommands,
	}, nil
}

func resolveSpaceTarget(spaces []*core.ResourceSpace) (string, []string) {
	for _, space := range spaces {
		if space == nil {
			continue
		}
		switch space.Kind {
		case core.ResourceKindLocalFS:
			if path := strings.TrimSpace(space.RootURI); path != "" {
				return path, readCheckCommands(space.Config)
			}
		case core.ResourceKindGit:
			if path := resolveGitSpacePath(space); path != "" {
				return path, readCheckCommands(space.Config)
			}
		}
	}
	return "", nil
}

func resolveGitSpacePath(space *core.ResourceSpace) string {
	if space == nil {
		return ""
	}
	if uri := strings.TrimSpace(space.RootURI); uri != "" && !looksLikeRemoteGitURI(uri) {
		return uri
	}
	if space.Config == nil {
		return ""
	}
	if cloneDir, ok := space.Config["clone_dir"].(string); ok {
		return strings.TrimSpace(cloneDir)
	}
	return ""
}

func looksLikeRemoteGitURI(uri string) bool {
	if strings.Contains(uri, "://") {
		return true
	}
	return strings.HasPrefix(uri, "git@") && strings.Contains(uri, ":")
}

func readCheckCommands(cfg map[string]any) []string {
	if len(cfg) == 0 {
		return nil
	}
	raw, ok := cfg["check_commands"]
	if !ok {
		return nil
	}
	switch value := raw.(type) {
	case []string:
		out := make([]string, 0, len(value))
		for _, item := range value {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			if s, ok := item.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	default:
		return nil
	}
}

func projectSlug(project *core.Project) string {
	if project == nil {
		return "project"
	}
	base := strings.ToLower(strings.TrimSpace(project.Name))
	if base == "" {
		base = "project-" + strconv.FormatInt(project.ID, 10)
	}
	base = strings.ReplaceAll(base, "_", "-")
	base = strings.ReplaceAll(base, " ", "-")
	base = slugSanitizer.ReplaceAllString(base, "-")
	for strings.Contains(base, "--") {
		base = strings.ReplaceAll(base, "--", "-")
	}
	base = strings.Trim(base, "-")
	if base == "" {
		base = "project-" + strconv.FormatInt(project.ID, 10)
	}
	return base
}

func nowUTC() time.Time {
	return time.Now().UTC()
}

func syncMountAliasDirs(mountsDir string, payload *core.ThreadWorkspaceContext, aliasTargets map[string]string) error {
	if strings.TrimSpace(mountsDir) == "" {
		return nil
	}
	entries, err := os.ReadDir(mountsDir)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(mountsDir, 0o755); err != nil {
				return err
			}
			entries = nil
		} else {
			return fmt.Errorf("read mounts dir: %w", err)
		}
	}
	keep := map[string]struct{}{}
	if payload != nil {
		for alias := range payload.Mounts {
			alias = strings.TrimSpace(alias)
			if alias == "" {
				continue
			}
			keep[alias] = struct{}{}
			if err := ensureMountAliasPath(filepath.Join(mountsDir, alias), aliasTargets[alias]); err != nil {
				return fmt.Errorf("create mount alias dir %q: %w", alias, err)
			}
		}
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, ok := keep[entry.Name()]; ok {
			continue
		}
		if err := os.RemoveAll(filepath.Join(mountsDir, entry.Name())); err != nil {
			return fmt.Errorf("remove stale mount alias dir %q: %w", entry.Name(), err)
		}
	}
	return nil
}

func resolvedMountTargets(ctx context.Context, store Store, threadID int64) map[string]string {
	if store == nil {
		return nil
	}
	refs, err := store.ListThreadContextRefs(ctx, threadID)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, ref := range refs {
		mount, err := ResolveMount(ctx, store, ref)
		if err != nil || mount == nil {
			continue
		}
		out[mount.Slug] = mount.TargetPath
	}
	return out
}

func ensureMountAliasPath(aliasPath string, targetPath string) error {
	if info, err := os.Lstat(aliasPath); err == nil && info.IsDir() {
		return nil
	}
	if runtime.GOOS == "windows" && strings.TrimSpace(targetPath) != "" {
		if err := os.MkdirAll(filepath.Dir(aliasPath), 0o755); err != nil {
			return err
		}
		cmd := exec.Command("cmd", "/c", "mklink", "/J", aliasPath, targetPath)
		if output, err := cmd.CombinedOutput(); err == nil {
			return nil
		} else if !strings.Contains(strings.ToLower(string(output)), "cannot create a file when that file already exists") {
			// Fall back to a plain directory if junction creation is unavailable.
		}
	}
	return os.MkdirAll(aliasPath, 0o755)
}
