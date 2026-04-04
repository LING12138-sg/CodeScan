package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteListFilesSkipsIgnoredDirectories(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "project"))
	mustMkdirAll(t, filepath.Join(root, "projects"))
	mustMkdirAll(t, filepath.Join(root, "src"))

	result := ExecuteListFiles(root, 100)

	if strings.Contains(result, "[D] project") || strings.Contains(result, "[D] projects") {
		t.Fatalf("expected ignored directories to be hidden, got %q", result)
	}
	if !strings.Contains(result, "[D] src") {
		t.Fatalf("expected non-ignored directory to remain visible, got %q", result)
	}
}

func TestRecursiveToolsSkipIgnoredDirectories(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "project", "ignored.go"), "package project\nconst marker = \"SKIP_ME\"\n")
	mustWriteFile(t, filepath.Join(root, "projects", "ignored.go"), "package projects\nconst marker = \"SKIP_ME_TOO\"\n")
	mustWriteFile(t, filepath.Join(root, "src", "keep.go"), "package src\nconst marker = \"KEEP_ME\"\n")

	treeResult := ExecuteListDirTree(root, 4, 100)
	if strings.Contains(treeResult, "project") || strings.Contains(treeResult, "projects") {
		t.Fatalf("expected tree listing to skip ignored directories, got %q", treeResult)
	}

	searchResult := ExecuteSearchFiles(root, "*.go", 100, 0)
	if strings.Contains(searchResult, filepath.Join("project", "ignored.go")) || strings.Contains(searchResult, filepath.Join("projects", "ignored.go")) {
		t.Fatalf("expected search to skip ignored directories, got %q", searchResult)
	}
	if !strings.Contains(searchResult, filepath.Join("src", "keep.go")) {
		t.Fatalf("expected search to include non-ignored files, got %q", searchResult)
	}

	grepResult := ExecuteGrepFiles(root, "SKIP_ME|KEEP_ME", false, 100, 0, 100, 32*1024)
	if strings.Contains(grepResult, filepath.Join("project", "ignored.go")) || strings.Contains(grepResult, filepath.Join("projects", "ignored.go")) {
		t.Fatalf("expected grep to skip ignored directories, got %q", grepResult)
	}
	if !strings.Contains(grepResult, filepath.Join("src", "keep.go")) {
		t.Fatalf("expected grep to include non-ignored files, got %q", grepResult)
	}
}

func TestResolveToolPathRejectsIgnoredDirectories(t *testing.T) {
	root := t.TempDir()

	_, err := resolveToolPath(root, filepath.Join("project", "ignored.go"))
	if err == nil || !strings.Contains(err.Error(), "ignored directory") {
		t.Fatalf("expected ignored directory error for project path, got %v", err)
	}

	_, err = resolveToolPath(root, filepath.Join("projects", "ignored.go"))
	if err == nil || !strings.Contains(err.Error(), "ignored directory") {
		t.Fatalf("expected ignored directory error for projects path, got %v", err)
	}

	resolved, err := resolveToolPath(root, filepath.Join("src", "keep.go"))
	if err != nil {
		t.Fatalf("expected non-ignored path to resolve, got %v", err)
	}
	if !strings.HasSuffix(resolved, filepath.Join("src", "keep.go")) {
		t.Fatalf("expected resolved path to point to src/keep.go, got %q", resolved)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path string, content string) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
