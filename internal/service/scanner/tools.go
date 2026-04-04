package scanner

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

// Tool Definitions
var Tools = []openai.Tool{
	{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "read_file",
			Description: "Read the content of a file from the local file system. Supports reading specific lines.",
			Parameters: jsonschema.Definition{
				Type: jsonschema.Object,
				Properties: map[string]jsonschema.Definition{
					"path": {
						Type:        jsonschema.String,
						Description: "The absolute or relative path to the file to read",
					},
					"start_line": {
						Type:        jsonschema.Integer,
						Description: "The line number to start reading from (1-based, optional)",
					},
					"end_line": {
						Type:        jsonschema.Integer,
						Description: "The line number to end reading at (1-based, optional)",
					},
					"max_output_bytes": {
						Type:        jsonschema.Integer,
						Description: "Maximum output bytes before truncation (optional)",
					},
				},
				Required: []string{"path"},
			},
		},
	},
	{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "get_evidence",
			Description: "Retrieve a previously preserved read_file snippet by evidence ID after context compression.",
			Parameters: jsonschema.Definition{
				Type: jsonschema.Object,
				Properties: map[string]jsonschema.Definition{
					"evidence_id": {
						Type:        jsonschema.String,
						Description: "The evidence ID from the preserved read_file evidence index.",
					},
				},
				Required: []string{"evidence_id"},
			},
		},
	},
	{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "get_artifact",
			Description: "Retrieve a previously preserved artifact by ID after context compression. Prefer this for any indexed artifact, including read_file snippets and older tool outputs.",
			Parameters: jsonschema.Definition{
				Type: jsonschema.Object,
				Properties: map[string]jsonschema.Definition{
					"artifact_id": {
						Type:        jsonschema.String,
						Description: "The artifact ID from the preserved artifact index.",
					},
				},
				Required: []string{"artifact_id"},
			},
		},
	},
	{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "list_files",
			Description: "List files and directories in a given path (non-recursive)",
			Parameters: jsonschema.Definition{
				Type: jsonschema.Object,
				Properties: map[string]jsonschema.Definition{
					"path": {
						Type:        jsonschema.String,
						Description: "The directory path to list. Defaults to current directory if empty.",
					},
					"max_entries": {
						Type:        jsonschema.Integer,
						Description: "Maximum number of entries to return (optional).",
					},
				},
				Required: []string{"path"},
			},
		},
	},
	{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "list_dir_tree",
			Description: "List directory structure recursively (tree view)",
			Parameters: jsonschema.Definition{
				Type: jsonschema.Object,
				Properties: map[string]jsonschema.Definition{
					"path": {
						Type:        jsonschema.String,
						Description: "The root directory path. Defaults to current directory if empty.",
					},
					"max_depth": {
						Type:        jsonschema.Integer,
						Description: "Maximum depth to traverse. Default is 2.",
					},
					"max_entries": {
						Type:        jsonschema.Integer,
						Description: "Maximum number of entries to return (optional).",
					},
				},
				Required: []string{"path"},
			},
		},
	},
	{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "search_files",
			Description: "Search for files by name pattern (Glob)",
			Parameters: jsonschema.Definition{
				Type: jsonschema.Object,
				Properties: map[string]jsonschema.Definition{
					"path": {
						Type:        jsonschema.String,
						Description: "The directory to search in. Defaults to current directory.",
					},
					"pattern": {
						Type:        jsonschema.String,
						Description: "The glob pattern (e.g., '*.go', 'config/*.json').",
					},
					"max_results": {
						Type:        jsonschema.Integer,
						Description: "Maximum number of results to return (optional).",
					},
					"offset": {
						Type:        jsonschema.Integer,
						Description: "Skip the first N matches (optional).",
					},
				},
				Required: []string{"pattern"},
			},
		},
	},
	{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name:        "grep_files",
			Description: "Search for text content within files using Regex",
			Parameters: jsonschema.Definition{
				Type: jsonschema.Object,
				Properties: map[string]jsonschema.Definition{
					"path": {
						Type:        jsonschema.String,
						Description: "The directory to search in. Defaults to current directory.",
					},
					"pattern": {
						Type:        jsonschema.String,
						Description: "The regex pattern to search for.",
					},
					"case_insensitive": {
						Type:        jsonschema.Boolean,
						Description: "Whether search should be case insensitive.",
					},
					"max_results": {
						Type:        jsonschema.Integer,
						Description: "Maximum number of matched lines to return (optional).",
					},
					"offset": {
						Type:        jsonschema.Integer,
						Description: "Skip the first N matches (optional).",
					},
					"max_files": {
						Type:        jsonschema.Integer,
						Description: "Maximum number of files to scan (optional).",
					},
					"max_output_bytes": {
						Type:        jsonschema.Integer,
						Description: "Maximum output bytes before truncation (optional).",
					},
				},
				Required: []string{"pattern"},
			},
		},
	},
}

const (
	defaultReadMaxOutputBytes   = 500 * 1024
	defaultGrepMaxOutputBytes   = 300 * 1024
	defaultListMaxOutputBytes   = 200 * 1024
	defaultSearchMaxOutputBytes = 200 * 1024
	defaultReadMaxLines         = 200
	defaultGrepMaxResults       = 200
	defaultSearchMaxResults     = 200
	defaultListMaxEntries       = 300
	defaultTreeMaxEntries       = 500
	maxGrepFileBytes            = 2 * 1024 * 1024
)

var stopWalk = errors.New("stop")

var skipDirNames = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	"vendor":       {},
	"dist":         {},
	"build":        {},
	"target":       {},
	"bin":          {},
	"obj":          {},
	".idea":        {},
	".vscode":      {},
	".next":        {},
	".nuxt":        {},
	".cache":       {},
	"coverage":     {},
	"project":      {},
	"projects":     {},
}

func GetStringArg(args map[string]interface{}, key string) string {
	if v, ok := args[key]; ok && v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func GetFloatArg(args map[string]interface{}, key string) float64 {
	if v, ok := args[key]; ok && v != nil {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return 0
}

func shouldSkipDir(info os.FileInfo) bool {
	if !info.IsDir() {
		return false
	}
	_, ok := skipDirNames[info.Name()]
	return ok
}

func pathHasSkippedComponent(path string) bool {
	cleanPath := filepath.Clean(path)
	for _, part := range strings.Split(cleanPath, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		if _, ok := skipDirNames[part]; ok {
			return true
		}
	}
	return false
}

func ExecuteReadFile(path string, startLine, endLine int, maxOutputBytes int) string {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Sprintf("Error reading file: %v", err)
	}
	defer f.Close()

	// Check file size
	_, err = f.Stat()
	if err != nil {
		return fmt.Sprintf("Error stating file: %v", err)
	}

	if maxOutputBytes <= 0 {
		maxOutputBytes = defaultReadMaxOutputBytes
	}

	autoRange := false
	if startLine <= 0 && endLine <= 0 {
		startLine = 1
		endLine = defaultReadMaxLines
		autoRange = true
	}

	// 2. Normalize arguments
	if startLine < 1 {
		startLine = 1
	}
	if endLine > 0 && startLine > endLine {
		return fmt.Sprintf("Error: start_line (%d) > end_line (%d)", startLine, endLine)
	}

	// 3. Scan lines
	scanner := bufio.NewScanner(f)
	// Buffer up to 1MB per line to handle minified code
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var sb strings.Builder
	currentLine := 0

	// Safety: if endLine is not set, limit to startLine + 1000
	if endLine <= 0 {
		endLine = startLine + 1000
	}

	for scanner.Scan() {
		currentLine++
		if currentLine >= startLine {
			sb.WriteString(scanner.Text())
			sb.WriteString("\n")
		}
		if currentLine >= endLine {
			break
		}
		if sb.Len() > maxOutputBytes {
			sb.WriteString("\n... (Output truncated) ...")
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Sprintf("Error scanning file: %v", err)
	}

	if sb.Len() == 0 && currentLine < startLine {
		return fmt.Sprintf("File is shorter than start_line (%d). Total lines: %d", startLine, currentLine)
	}

	if autoRange {
		sb.WriteString(fmt.Sprintf("\n... (Partial output: lines %d-%d. Use start_line/end_line for more.)", startLine, endLine))
	}

	return sb.String()
}

func ExecuteListFiles(path string, maxEntries int) string {
	files, err := os.ReadDir(path)
	if err != nil {
		return fmt.Sprintf("Error listing files: %v", err)
	}

	if maxEntries <= 0 {
		maxEntries = defaultListMaxEntries
	}

	var sb strings.Builder
	count := 0
	for _, f := range files {
		info, _ := f.Info()
		if shouldSkipDir(info) {
			continue
		}
		prefix := "F"
		if f.IsDir() {
			prefix = "D"
		}
		sb.WriteString(fmt.Sprintf("[%s] %s (%d bytes)\n", prefix, f.Name(), info.Size()))
		count++
		if maxEntries > 0 && count >= maxEntries {
			sb.WriteString("... (List truncated) ...\n")
			break
		}
		if sb.Len() > defaultListMaxOutputBytes {
			sb.WriteString("... (Output truncated) ...\n")
			break
		}
	}
	if sb.Len() == 0 {
		return "Directory is empty."
	}
	return sb.String()
}

func ExecuteListDirTree(path string, maxDepth int, maxEntries int) string {
	var sb strings.Builder
	rootDepth := strings.Count(filepath.Clean(path), string(os.PathSeparator))
	count := 0

	if maxEntries <= 0 {
		maxEntries = defaultTreeMaxEntries
	}

	err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if shouldSkipDir(info) {
			return filepath.SkipDir
		}

		currentDepth := strings.Count(p, string(os.PathSeparator)) - rootDepth
		if currentDepth < 0 {
			currentDepth = 0
		}
		if currentDepth > maxDepth {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		indent := strings.Repeat("  ", currentDepth)
		prefix := "F"
		if info.IsDir() {
			prefix = "D"
		}
		sb.WriteString(fmt.Sprintf("%s[%s] %s\n", indent, prefix, info.Name()))
		count++
		if maxEntries > 0 && count >= maxEntries {
			sb.WriteString("... (Tree truncated) ...\n")
			return stopWalk
		}
		if sb.Len() > defaultListMaxOutputBytes {
			sb.WriteString("... (Output truncated) ...\n")
			return stopWalk
		}
		return nil
	})

	if err != nil && !errors.Is(err, stopWalk) {
		return fmt.Sprintf("Error walking directory: %v", err)
	}
	return sb.String()
}

func ExecuteSearchFiles(path string, pattern string, maxResults int, offset int) string {
	var matches []string
	if maxResults <= 0 {
		maxResults = defaultSearchMaxResults
	}
	if os.PathSeparator != '/' {
		pattern = strings.ReplaceAll(pattern, "/", string(os.PathSeparator))
	}
	prefix := "**" + string(os.PathSeparator)
	if strings.HasPrefix(pattern, prefix) {
		pattern = strings.TrimPrefix(pattern, prefix)
	}
	hasDir := strings.Contains(pattern, string(os.PathSeparator))

	err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if shouldSkipDir(info) {
			return filepath.SkipDir
		}

		var nameToMatch string
		if hasDir {
			relPath, err := filepath.Rel(path, p)
			if err != nil {
				return nil
			}
			nameToMatch = relPath
		} else {
			nameToMatch = info.Name()
		}

		match, err := filepath.Match(pattern, nameToMatch)
		if err != nil {
			return err
		}

		if match {
			relPath, _ := filepath.Rel(path, p)
			matches = append(matches, relPath)
			if maxResults > 0 && len(matches) >= maxResults+offset {
				return stopWalk
			}
		}
		return nil
	})

	if err != nil && !errors.Is(err, stopWalk) {
		return fmt.Sprintf("Error searching files: %v", err)
	}
	if len(matches) == 0 {
		return "No matching files found."
	}
	if offset > 0 && offset < len(matches) {
		matches = matches[offset:]
	} else if offset >= len(matches) {
		return "No matching files found."
	}
	if maxResults > 0 && len(matches) > maxResults {
		matches = matches[:maxResults]
	}
	result := strings.Join(matches, "\n")
	if len(result) > defaultSearchMaxOutputBytes {
		return result[:defaultSearchMaxOutputBytes] + "\n... (Output truncated) ..."
	}
	return result
}

func ExecuteGrepFiles(path string, pattern string, caseInsensitive bool, maxResults int, offset int, maxFiles int, maxOutputBytes int) string {
	var sb strings.Builder
	prefix := ""
	if caseInsensitive {
		prefix = "(?i)"
	}

	re, err := regexp.Compile(prefix + pattern)
	if err != nil {
		return fmt.Sprintf("Invalid regex pattern: %v", err)
	}

	if maxResults <= 0 {
		maxResults = defaultGrepMaxResults
	}
	if maxFiles <= 0 {
		maxFiles = 1000
	}
	if maxOutputBytes <= 0 {
		maxOutputBytes = defaultGrepMaxOutputBytes
	}

	matchedCount := 0
	writtenCount := 0
	scannedFiles := 0

	err = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if shouldSkipDir(info) {
				return filepath.SkipDir
			}
			return nil
		}
		scannedFiles++
		if scannedFiles > maxFiles {
			sb.WriteString("... (File scan limit reached) ...\n")
			return stopWalk
		}

		if info.Size() > maxGrepFileBytes {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(p))
		if ext == ".exe" || ext == ".dll" || ext == ".bin" || ext == ".git" {
			return nil
		}
		if strings.Contains(p, ".git"+string(os.PathSeparator)) {
			return nil
		}

		content, err := os.ReadFile(p)
		if err != nil {
			return nil
		}

		// Use bufio.Scanner for line-by-line matching to reduce memory allocation
		lineNum := 0
		lineScanner := bufio.NewScanner(strings.NewReader(string(content)))
		lineBuf := make([]byte, 0, 64*1024)
		lineScanner.Buffer(lineBuf, 1024*1024)
		for lineScanner.Scan() {
			lineNum++
			line := lineScanner.Text()
			if re.MatchString(line) {
				matchedCount++
				if matchedCount <= offset {
					continue
				}
				relPath, _ := filepath.Rel(path, p)
				displayLine := strings.TrimSpace(line)
				if len(displayLine) > 100 {
					displayLine = displayLine[:100] + "..."
				}
				sb.WriteString(fmt.Sprintf("%s:%d: %s\n", relPath, lineNum, displayLine))
				writtenCount++
				if writtenCount >= maxResults {
					sb.WriteString("... (Results truncated) ...\n")
					return stopWalk
				}
				if sb.Len() > maxOutputBytes {
					sb.WriteString("... (Output truncated) ...\n")
					return stopWalk
				}
			}
		}
		return nil
	})

	if err != nil && !errors.Is(err, stopWalk) {
		return fmt.Sprintf("Error walking directory: %v", err)
	}
	result := sb.String()
	if result == "" {
		return "No matches found."
	}
	return result
}
