package scanner

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"codescan/internal/database"
	"codescan/internal/model"
)

type StaticFinding struct {
	Type           string            `json:"type"`
	Subtype        string            `json:"subtype"`
	Severity       string            `json:"severity"`
	Location       map[string]string `json:"location"`
	Trigger        map[string]string `json:"trigger"`
	Description    string            `json:"description"`
	VulnerableCode string            `json:"vulnerable_code"`
}

var staticRules = []struct {
	Pattern  *regexp.Regexp
	Type     string
	Subtype  string
	Severity string
	Desc     string
}{
	{
		Pattern:  regexp.MustCompile(`(?i)(system|exec|passthru|shell_exec|eval|popen)\s*\(`),
		Type:     "RCE",
		Subtype:  "Command Injection",
		Severity: "CRITICAL",
		Desc:     "Detected potential command execution function / 发现潜在的命令执行函数",
	},
	{
		Pattern:  regexp.MustCompile(`(?i)(mysql_query|mysqli_query|PDO::query)\s*\(`),
		Type:     "Injection",
		Subtype:  "SQL Injection",
		Severity: "HIGH",
		Desc:     "Detected direct database query which might be vulnerable to SQL injection / 发现直接的数据库查询，可能存在SQL注入",
	},
	{
		Pattern:  regexp.MustCompile(`(?i)(md5|sha1)\s*\(`),
		Type:     "Configuration",
		Subtype:  "Weak Cryptography",
		Severity: "MEDIUM",
		Desc:     "Detected weak cryptographic hash function / 发现弱密码哈希函数",
	},
}

func RunStaticScan(task *model.Task) {
	stageName := "static_scan"

	// Recover from panics
	defer func() {
		if r := recover(); r != nil {
			task.Status = "failed"
			database.DB.Save(task)
		}
	}()

	task.BasePath = task.GetBasePath()

	// Initialize stage
	var stage model.TaskStage
	err := database.DB.Where("task_id = ? AND name = ?", task.ID, stageName).First(&stage).Error
	if err != nil {
		stage = model.TaskStage{
			TaskID:     task.ID,
			Name:       stageName,
			Status:     "running",
			CreatedAt:  time.Now(),
			Logs:       []string{"[System] Starting basic static code scan..."},
			OutputJSON: json.RawMessage("[]"),
		}
		database.DB.Create(&stage)
	} else {
		stage.Status = "running"
		stage.Logs = []string{"[System] Starting basic static code scan..."}
		database.DB.Save(&stage)
	}

	findings := []StaticFinding{}

	err = filepath.WalkDir(task.BasePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext != ".php" && ext != ".go" && ext != ".js" && ext != ".java" && ext != ".py" && ext != ".jsp" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(task.BasePath, path)
		lines := strings.Split(string(content), "\n")

		for i, line := range lines {
			if len(line) > 500 { // skip very long lines minified code
				continue
			}
			for _, rule := range staticRules {
				if rule.Pattern.MatchString(line) {
					findings = append(findings, StaticFinding{
						Type:     rule.Type,
						Subtype:  rule.Subtype,
						Severity: rule.Severity,
						Location: map[string]string{
							"file": filepath.ToSlash(relPath),
							"line": fmt.Sprintf("%d", i+1),
						},
						Trigger: map[string]string{
							"method": "",
							"path":   "",
						},
						Description:    rule.Desc,
						VulnerableCode: strings.TrimSpace(line),
					})
				}
			}
		}
		return nil
	})

	if err != nil {
		stage.Status = "failed"
		stage.Logs = append(stage.Logs, fmt.Sprintf("[Error] Scan failed: %v", err))
		database.DB.Save(&stage)
		checkAndUpdateTaskStatus(task)
		return
	}

	outBytes, _ := json.MarshalIndent(findings, "", "  ")
	stage.OutputJSON = json.RawMessage(outBytes)
	stage.Status = "completed"
	stage.Logs = append(stage.Logs, fmt.Sprintf("[System] Static scan completed. Found %d issues.", len(findings)))
	database.DB.Save(&stage)

	checkAndUpdateTaskStatus(task)
}

func checkAndUpdateTaskStatus(task *model.Task) {
	var runningCount int64
	database.DB.Model(&model.TaskStage{}).Where("task_id = ? AND status = ?", task.ID, "running").Count(&runningCount)
	if runningCount == 0 {
		database.DB.Model(task).Update("status", "completed")
	}
}
