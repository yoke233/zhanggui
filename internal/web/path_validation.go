package web

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

var (
	errRelativePathRequired = errors.New("relative path is required")
	errInvalidRelativePath  = errors.New("invalid relative path")
)

type relativePathValidationError struct {
	kind    error
	message string
}

func (e *relativePathValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

func (e *relativePathValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.kind
}

func validateRelativePath(repoRoot, rawPath string) (string, string, error) {
	trimmedRepoRoot := strings.TrimSpace(repoRoot)
	if trimmedRepoRoot == "" {
		return "", "", &relativePathValidationError{
			kind:    errInvalidRelativePath,
			message: "invalid repository root path",
		}
	}

	absRepoRoot, err := filepath.Abs(trimmedRepoRoot)
	if err != nil {
		return "", "", &relativePathValidationError{
			kind:    errInvalidRelativePath,
			message: "invalid repository root path",
		}
	}

	trimmedPath := strings.TrimSpace(rawPath)
	if trimmedPath == "" {
		return "", "", &relativePathValidationError{
			kind:    errRelativePathRequired,
			message: "relative path is required",
		}
	}

	normalizedInput := strings.ReplaceAll(trimmedPath, "\\", "/")
	cleanRelative := filepath.Clean(filepath.FromSlash(normalizedInput))
	if filepath.IsAbs(cleanRelative) || filepath.VolumeName(cleanRelative) != "" || hasWindowsDrivePrefix(cleanRelative) {
		return "", "", &relativePathValidationError{
			kind:    errInvalidRelativePath,
			message: fmt.Sprintf("invalid relative path %q", trimmedPath),
		}
	}

	absPath := filepath.Join(absRepoRoot, cleanRelative)
	relPath, err := filepath.Rel(absRepoRoot, absPath)
	if err != nil {
		return "", "", &relativePathValidationError{
			kind:    errInvalidRelativePath,
			message: fmt.Sprintf("invalid relative path %q", trimmedPath),
		}
	}
	if relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
		return "", "", &relativePathValidationError{
			kind:    errInvalidRelativePath,
			message: fmt.Sprintf("invalid relative path %q", trimmedPath),
		}
	}

	return absPath, filepath.ToSlash(relPath), nil
}

func hasWindowsDrivePrefix(value string) bool {
	if len(value) < 2 {
		return false
	}
	drive := value[0]
	return ((drive >= 'A' && drive <= 'Z') || (drive >= 'a' && drive <= 'z')) && value[1] == ':'
}
