package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/yoke233/zhanggui/internal/uuidv7"
)

type Action string

const (
	ActionCreate  Action = "create"
	ActionAppend  Action = "append"
	ActionReplace Action = "replace"
	ActionMkdir   Action = "mkdir"
	ActionRename  Action = "rename"
	ActionDelete  Action = "delete"
)

type Actor struct {
	AgentID string `json:"agent_id"`
	Role    string `json:"role"`
}

type Linkage struct {
	ThreadID  string `json:"thread_id,omitempty"`
	MeetingID string `json:"meeting_id,omitempty"`
	TaskID    string `json:"task_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Rev       string `json:"rev,omitempty"`
}

type AuditRecord struct {
	SchemaVersion int         `json:"schema_version"`
	TS            string      `json:"ts"`
	Who           Actor       `json:"who"`
	What          AuditWhat   `json:"what"`
	Where         AuditWhere  `json:"where"`
	Result        AuditResult `json:"result"`
	Linkage       Linkage     `json:"linkage,omitempty"`
}

type AuditWhat struct {
	Action Action `json:"action"`
	Tool   string `json:"tool"`
	Detail string `json:"detail,omitempty"`
}

type AuditWhere struct {
	Path string `json:"path"`
	To   string `json:"to,omitempty"`
}

type AuditResult struct {
	Status    string `json:"status"`
	ErrorCode string `json:"error_code,omitempty"`
	Error     string `json:"error,omitempty"`
}

type Policy struct {
	AllowedWritePrefixes []string
	AppendOnlyFiles      []string
	SingleWriterPrefixes []string
	SingleWriterRoles    []string
	LockFile             string

	AllowRename bool
	AllowDelete bool
}

type Gateway struct {
	rootAbs string
	actor   Actor
	linkage Linkage

	allowedPrefixes      []string
	appendOnlyFiles      map[string]struct{}
	singleWriterPrefixes []string
	singleWriterRoles    map[string]struct{}
	lockFileRel          string
	lockFileAbs          string
	lockHeld             bool

	allowRename bool
	allowDelete bool

	auditor *Auditor
}

func New(rootDir string, actor Actor, linkage Linkage, policy Policy, auditor *Auditor) (*Gateway, error) {
	if rootDir == "" {
		return nil, fmt.Errorf("rootDir 不能为空")
	}
	rootAbs, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, err
	}
	if auditor == nil {
		return nil, fmt.Errorf("auditor 不能为空（审计为硬要求）")
	}

	allowed, err := normalizePrefixes(policy.AllowedWritePrefixes)
	if err != nil {
		return nil, err
	}
	appendOnly := make(map[string]struct{}, len(policy.AppendOnlyFiles))
	for _, f := range policy.AppendOnlyFiles {
		n, err := normalizeRelPath(f)
		if err != nil {
			return nil, fmt.Errorf("append_only_files 包含非法路径 %q: %w", f, err)
		}
		appendOnly[n] = struct{}{}
	}

	swp, err := normalizePrefixes(policy.SingleWriterPrefixes)
	if err != nil {
		return nil, err
	}
	swr := make(map[string]struct{}, len(policy.SingleWriterRoles))
	for _, r := range policy.SingleWriterRoles {
		if strings.TrimSpace(r) == "" {
			continue
		}
		swr[strings.ToLower(strings.TrimSpace(r))] = struct{}{}
	}

	lockRel := ""
	lockAbs := ""
	if policy.LockFile != "" {
		lockRel, err = normalizeRelPath(policy.LockFile)
		if err != nil {
			return nil, fmt.Errorf("lock_file 非法: %w", err)
		}
		lockAbs = filepath.Join(rootAbs, filepath.FromSlash(lockRel))
	}

	return &Gateway{
		rootAbs:              rootAbs,
		actor:                actor,
		linkage:              linkage,
		allowedPrefixes:      allowed,
		appendOnlyFiles:      appendOnly,
		singleWriterPrefixes: swp,
		singleWriterRoles:    swr,
		lockFileRel:          lockRel,
		lockFileAbs:          lockAbs,
		allowRename:          policy.AllowRename,
		allowDelete:          policy.AllowDelete,
		auditor:              auditor,
	}, nil
}

func (g *Gateway) Root() string { return g.rootAbs }

func (g *Gateway) AcquireLock() error {
	if g.lockFileAbs == "" {
		return deny("E_LOCK_DISABLED", "lock_file 未配置")
	}
	if g.lockHeld {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(g.lockFileAbs), 0o755); err != nil {
		return err
	}
	payload := map[string]any{
		"schema_version": 1,
		"lock_id":        uuidv7.New(),
		"actor":          g.actor,
		"acquired_at":    time.Now().Format(time.RFC3339),
		"linkage":        g.linkage,
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	f, err := os.OpenFile(g.lockFileAbs, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return deny("E_LOCK_HELD", fmt.Sprintf("lock 已存在: %s", filepath.ToSlash(g.lockFileRel)))
		}
		return err
	}
	_, werr := f.Write(b)
	cerr := f.Close()
	if werr != nil {
		_ = os.Remove(g.lockFileAbs)
		return werr
	}
	if cerr != nil {
		_ = os.Remove(g.lockFileAbs)
		return cerr
	}
	g.lockHeld = true
	return nil
}

func (g *Gateway) ReleaseLock() error {
	if g.lockFileAbs == "" {
		return nil
	}
	if !g.lockHeld {
		return nil
	}
	g.lockHeld = false
	return os.Remove(g.lockFileAbs)
}

func (g *Gateway) MkdirAll(relDir string, perm os.FileMode, detail string) error {
	return g.do(ActionMkdir, relDir, "", detail, func(abs string) error {
		return os.MkdirAll(abs, perm)
	})
}

func (g *Gateway) CreateFile(relPath string, data []byte, perm os.FileMode, detail string) error {
	return g.do(ActionCreate, relPath, "", detail, func(abs string) error {
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		if err != nil {
			return err
		}
		_, werr := f.Write(data)
		cerr := f.Close()
		if werr != nil {
			_ = os.Remove(abs)
			return werr
		}
		if cerr != nil {
			_ = os.Remove(abs)
			return cerr
		}
		return nil
	})
}

func (g *Gateway) CreateFileFromReader(relPath string, perm os.FileMode, detail string, r io.Reader) error {
	if r == nil {
		return fmt.Errorf("reader 不能为空")
	}
	return g.do(ActionCreate, relPath, "", detail, func(abs string) error {
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		if err != nil {
			return err
		}
		_, werr := io.Copy(f, r)
		cerr := f.Close()
		if werr != nil {
			_ = os.Remove(abs)
			return werr
		}
		if cerr != nil {
			_ = os.Remove(abs)
			return cerr
		}
		return nil
	})
}

func (g *Gateway) ReplaceFile(relPath string, data []byte, perm os.FileMode, detail string) error {
	return g.do(ActionReplace, relPath, "", detail, func(abs string) error {
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		return writeFileAtomic(abs, data, perm)
	})
}

func (g *Gateway) ReplaceFileWith(relPath string, perm os.FileMode, detail string, writeTmp func(tmpAbs string) error) error {
	if writeTmp == nil {
		return fmt.Errorf("writeTmp 不能为空")
	}
	abs, norm, err := g.authorize(ActionReplace, relPath)
	if err != nil {
		return g.auditAuthError(ActionReplace, relPath, "", err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		if aerr := g.audit(ActionReplace, norm, "", detail, "error", codeOf(err), err.Error()); aerr != nil {
			return fmt.Errorf("%w; audit_failed: %v", err, aerr)
		}
		return err
	}

	dir := filepath.Dir(abs)
	base := filepath.Base(abs)
	tmp := filepath.Join(dir, "."+base+".tmp."+time.Now().Format("150405.000000000"))
	if err := writeTmp(tmp); err != nil {
		_ = os.Remove(tmp)
		if aerr := g.audit(ActionReplace, norm, "", detail, "error", codeOf(err), err.Error()); aerr != nil {
			return fmt.Errorf("%w; audit_failed: %v", err, aerr)
		}
		return err
	}

	_ = os.Remove(abs)
	if err := os.Rename(tmp, abs); err != nil {
		_ = os.Remove(tmp)
		if aerr := g.audit(ActionReplace, norm, "", detail, "error", codeOf(err), err.Error()); aerr != nil {
			return fmt.Errorf("%w; audit_failed: %v", err, aerr)
		}
		return err
	}
	if err := g.audit(ActionReplace, norm, "", detail, "ok", "", ""); err != nil {
		return err
	}
	return nil
}

func (g *Gateway) CreateFileWith(relPath string, perm os.FileMode, detail string, writeTmp func(tmpAbs string) error) error {
	if writeTmp == nil {
		return fmt.Errorf("writeTmp 不能为空")
	}
	abs, norm, err := g.authorize(ActionCreate, relPath)
	if err != nil {
		return g.auditAuthError(ActionCreate, relPath, "", err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		if aerr := g.audit(ActionCreate, norm, "", detail, "error", codeOf(err), err.Error()); aerr != nil {
			return fmt.Errorf("%w; audit_failed: %v", err, aerr)
		}
		return err
	}
	if _, err := os.Stat(abs); err == nil {
		// 保持 create-only：若目标已存在，直接报已存在。
		existErr := &os.PathError{Op: "create", Path: abs, Err: os.ErrExist}
		if aerr := g.audit(ActionCreate, norm, "", detail, "error", codeOf(existErr), existErr.Error()); aerr != nil {
			return fmt.Errorf("%w; audit_failed: %v", existErr, aerr)
		}
		return existErr
	} else if err != nil && !os.IsNotExist(err) {
		if aerr := g.audit(ActionCreate, norm, "", detail, "error", codeOf(err), err.Error()); aerr != nil {
			return fmt.Errorf("%w; audit_failed: %v", err, aerr)
		}
		return err
	}

	dir := filepath.Dir(abs)
	base := filepath.Base(abs)
	tmp := filepath.Join(dir, "."+base+".tmp."+time.Now().Format("150405.000000000"))
	if err := writeTmp(tmp); err != nil {
		_ = os.Remove(tmp)
		if aerr := g.audit(ActionCreate, norm, "", detail, "error", codeOf(err), err.Error()); aerr != nil {
			return fmt.Errorf("%w; audit_failed: %v", err, aerr)
		}
		return err
	}

	// Windows: Rename 到已存在目标会失败；create-only 的竞争态也会被正确拒绝。
	if err := os.Rename(tmp, abs); err != nil {
		_ = os.Remove(tmp)
		if aerr := g.audit(ActionCreate, norm, "", detail, "error", codeOf(err), err.Error()); aerr != nil {
			return fmt.Errorf("%w; audit_failed: %v", err, aerr)
		}
		return err
	}
	if err := os.Chmod(abs, perm); err != nil {
		// best-effort（Windows 可能不生效）；不阻断主流程。
	}
	if err := g.audit(ActionCreate, norm, "", detail, "ok", "", ""); err != nil {
		return err
	}
	return nil
}

func (g *Gateway) AppendFile(relPath string, data []byte, perm os.FileMode, detail string) error {
	return g.do(ActionAppend, relPath, "", detail, func(abs string) error {
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(abs, os.O_CREATE|os.O_APPEND|os.O_WRONLY, perm)
		if err != nil {
			return err
		}
		_, werr := f.Write(data)
		cerr := f.Close()
		if werr != nil {
			return werr
		}
		return cerr
	})
}

func (g *Gateway) Rename(fromRel, toRel, detail string) error {
	if !g.allowRename {
		return g.auditDeny(ActionRename, fromRel, toRel, "E_ACTION_DISABLED", "rename 默认禁用")
	}
	fromAbs, fromNorm, err := g.authorize(ActionRename, fromRel)
	if err != nil {
		return g.auditAuthError(ActionRename, fromRel, toRel, err)
	}
	toAbs, toNorm, err := g.authorize(ActionRename, toRel)
	if err != nil {
		return g.auditAuthError(ActionRename, fromRel, toRel, err)
	}

	err = os.Rename(fromAbs, toAbs)
	if err != nil {
		if aerr := g.audit(ActionRename, fromNorm, toNorm, detail, "error", codeOf(err), err.Error()); aerr != nil {
			return fmt.Errorf("%w; audit_failed: %v", err, aerr)
		}
		return err
	}
	if err := g.audit(ActionRename, fromNorm, toNorm, detail, "ok", "", ""); err != nil {
		return err
	}
	return nil
}

func (g *Gateway) Delete(relPath, detail string) error {
	if !g.allowDelete {
		return g.auditDeny(ActionDelete, relPath, "", "E_ACTION_DISABLED", "delete 默认禁用")
	}
	return g.do(ActionDelete, relPath, "", detail, func(abs string) error {
		return os.Remove(abs)
	})
}

func (g *Gateway) do(action Action, relPath string, toRel string, detail string, fn func(abs string) error) error {
	abs, norm, err := g.authorize(action, relPath)
	if err != nil {
		return g.auditAuthError(action, relPath, toRel, err)
	}
	if err := fn(abs); err != nil {
		if aerr := g.audit(action, norm, "", detail, "error", codeOf(err), err.Error()); aerr != nil {
			return fmt.Errorf("%w; audit_failed: %v", err, aerr)
		}
		return err
	}
	if err := g.audit(action, norm, "", detail, "ok", "", ""); err != nil {
		return err
	}
	return nil
}

func (g *Gateway) auditAuthError(action Action, relPath string, toRel string, err error) error {
	code := codeOf(err)
	if aerr := g.audit(action, sanitizeForAudit(relPath), sanitizeForAudit(toRel), "", "error", code, err.Error()); aerr != nil {
		return fmt.Errorf("%w; audit_failed: %v", err, aerr)
	}
	return err
}

func (g *Gateway) auditDeny(action Action, relPath string, toRel string, code string, msg string) error {
	derr := deny(code, msg)
	if aerr := g.audit(action, sanitizeForAudit(relPath), sanitizeForAudit(toRel), "", "error", code, msg); aerr != nil {
		return fmt.Errorf("%w; audit_failed: %v", derr, aerr)
	}
	return derr
}

func (g *Gateway) audit(action Action, rel string, to string, detail string, status string, errCode string, errMsg string) error {
	rec := AuditRecord{
		SchemaVersion: 1,
		TS:            time.Now().Format(time.RFC3339),
		Who:           g.actor,
		What:          AuditWhat{Action: action, Tool: "fs.write", Detail: detail},
		Where:         AuditWhere{Path: rel, To: to},
		Result:        AuditResult{Status: status, ErrorCode: errCode, Error: errMsg},
		Linkage:       g.linkage,
	}
	return g.auditor.Write(rec)
}

func (g *Gateway) authorize(action Action, relPath string) (abs string, relNorm string, err error) {
	if action == ActionRename || action == ActionDelete {
		// rename/delete 的 allow 检查在外层函数做，这里只管路径/语义。
	}
	relNorm, err = normalizeRelPath(relPath)
	if err != nil {
		return "", "", deny("E_BAD_PATH", err.Error())
	}

	if len(g.allowedPrefixes) == 0 {
		return "", "", deny("E_ACL_NO_PREFIXES", "allowed_write_prefixes 为空（默认拒绝所有写入）")
	}
	if !matchAnyPrefix(relNorm, g.allowedPrefixes) {
		return "", "", deny("E_ACL_DENY", fmt.Sprintf("path 不在允许前缀内: %s", relNorm))
	}

	abs = filepath.Join(g.rootAbs, filepath.FromSlash(relNorm))
	if !isUnderRoot(g.rootAbs, abs) {
		return "", "", deny("E_PATH_ESCAPE", fmt.Sprintf("path 逃逸 root: %s", relNorm))
	}

	if _, ok := g.appendOnlyFiles[relNorm]; ok {
		switch action {
		case ActionAppend:
			// ok
		case ActionCreate:
			if _, statErr := os.Stat(abs); statErr == nil {
				return "", "", deny("E_APPEND_ONLY_VIOLATION", fmt.Sprintf("append-only 已存在，禁止 create 覆盖: %s", relNorm))
			}
		default:
			return "", "", deny("E_APPEND_ONLY_VIOLATION", fmt.Sprintf("append-only 禁止动作 %s: %s", action, relNorm))
		}
	}

	if matchAnyPrefix(relNorm, g.singleWriterPrefixes) {
		if len(g.singleWriterRoles) > 0 {
			if _, ok := g.singleWriterRoles[strings.ToLower(strings.TrimSpace(g.actor.Role))]; !ok {
				return "", "", deny("E_SINGLE_WRITER_VIOLATION", fmt.Sprintf("role 无权写 single-writer 区域: role=%s path=%s", g.actor.Role, relNorm))
			}
		}
		if g.lockFileAbs != "" && !g.lockHeld {
			return "", "", deny("E_LOCK_NOT_HELD", fmt.Sprintf("未持有单写者锁: %s", filepath.ToSlash(g.lockFileRel)))
		}
	}

	return abs, relNorm, nil
}

func normalizeRelPath(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", fmt.Errorf("path 不能为空")
	}
	if strings.Contains(p, "\x00") {
		return "", fmt.Errorf("path 包含 NUL")
	}
	if filepath.IsAbs(p) {
		return "", fmt.Errorf("禁止绝对路径: %s", filepath.ToSlash(p))
	}
	// Windows: "C:foo" 不是绝对路径，但包含 drive 语义，统一拒绝。
	if strings.Contains(p, ":") {
		return "", fmt.Errorf("禁止包含 ':' 的路径: %s", filepath.ToSlash(p))
	}

	s := filepath.ToSlash(p)
	s = path.Clean(s)
	if s == "." {
		return "", fmt.Errorf("非法路径: %s", filepath.ToSlash(p))
	}
	if strings.HasPrefix(s, "/") {
		return "", fmt.Errorf("禁止以 '/' 开头: %s", s)
	}
	if s == ".." || strings.HasPrefix(s, "../") {
		return "", fmt.Errorf("禁止路径逃逸: %s", s)
	}
	return s, nil
}

func normalizePrefixes(prefixes []string) ([]string, error) {
	out := make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		p = strings.TrimSpace(p)
		if p == "" {
			out = append(out, "")
			continue
		}
		n, err := normalizeRelPath(p)
		if err != nil {
			return nil, fmt.Errorf("prefix 非法 %q: %w", p, err)
		}
		if n == "." {
			n = ""
		}
		out = append(out, n)
	}
	return out, nil
}

func matchAnyPrefix(rel string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return false
	}
	for _, p := range prefixes {
		if p == "" {
			return true
		}
		if rel == p {
			return true
		}
		if strings.HasPrefix(rel, p+"/") {
			return true
		}
	}
	return false
}

func isUnderRoot(rootAbs, targetAbs string) bool {
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, "../")
}

func writeFileAtomic(abs string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(abs)
	base := filepath.Base(abs)
	tmp := filepath.Join(dir, "."+base+".tmp")
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	_ = os.Remove(abs)
	if err := os.Rename(tmp, abs); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("atomic rename failed: %w", err)
	}
	return nil
}

func codeOf(err error) string {
	if err == nil {
		return ""
	}
	var de DenyError
	if errors.As(err, &de) && de.Code != "" {
		return de.Code
	}
	return ""
}

func sanitizeForAudit(p string) string {
	if strings.TrimSpace(p) == "" {
		return ""
	}
	s := filepath.ToSlash(p)
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "")
	return s
}
