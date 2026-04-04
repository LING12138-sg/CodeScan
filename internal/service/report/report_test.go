package report

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"codescan/internal/model"
)

func TestBuildIncludesOnlyCompletedStages(t *testing.T) {
	task := model.Task{
		ID:        "task12345678",
		Name:      "Demo Audit",
		Remark:    "report verification",
		CreatedAt: time.Date(2026, 3, 30, 9, 0, 0, 0, time.UTC),
		OutputJSON: json.RawMessage(`[
            {"method":"GET","path":"/api/users","source":"routes/user.go","description":"List users"},
            {"method":"POST","path":"/api/login","source":"routes/auth.go","description":"Login"}
        ]`),
		Stages: []model.TaskStage{
			{
				Name:       "rce",
				Status:     "completed",
				UpdatedAt:  time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC),
				OutputJSON: json.RawMessage(`[{"type":"RCE","subtype":"Command Injection","severity":"CRITICAL","description":"Unsanitized shell exec","location":{"file":"cmd/app.go","line":88},"trigger":{"method":"POST","path":"/api/run","parameter":"cmd"},"execution_logic":"user input reaches exec","vulnerable_code":"exec.Command(input)","impact":"Remote command execution"}]`),
			},
			{
				Name:       "xss",
				Status:     "running",
				UpdatedAt:  time.Date(2026, 3, 30, 10, 5, 0, 0, time.UTC),
				OutputJSON: json.RawMessage(`[{"type":"XSS","subtype":"Stored XSS"}]`),
			},
			{
				Name:       "auth",
				Status:     "completed",
				UpdatedAt:  time.Date(2026, 3, 30, 10, 10, 0, 0, time.UTC),
				OutputJSON: json.RawMessage(`[]`),
			},
		},
	}

	report, err := Build(task, time.Date(2026, 3, 30, 11, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(report.Stages) != 2 {
		t.Fatalf("expected 2 completed/exportable stages, got %d", len(report.Stages))
	}
	if report.Stages[0].Key != "rce" || report.Stages[1].Key != "auth" {
		t.Fatalf("unexpected stage order: %+v", report.Stages)
	}
	if report.Summary.CompletedStageCount != 2 {
		t.Fatalf("expected CompletedStageCount=2, got %d", report.Summary.CompletedStageCount)
	}
	if report.Summary.TotalFindings != 1 {
		t.Fatalf("expected TotalFindings=1, got %d", report.Summary.TotalFindings)
	}
	if report.Summary.CleanStageCount != 1 {
		t.Fatalf("expected CleanStageCount=1, got %d", report.Summary.CleanStageCount)
	}
	if report.Summary.RouteCount != 2 {
		t.Fatalf("expected RouteCount=2, got %d", report.Summary.RouteCount)
	}
	if report.Stages[1].ZeroFindings != true {
		t.Fatalf("expected auth stage to be marked zero findings")
	}
}

func TestBuildFallsBackToRawResultWhenJSONMissing(t *testing.T) {
	task := model.Task{
		ID:        "taskraw123456",
		Name:      "Raw Audit",
		CreatedAt: time.Date(2026, 3, 30, 9, 0, 0, 0, time.UTC),
		Stages: []model.TaskStage{
			{
				Name:      "logic",
				Status:    "completed",
				UpdatedAt: time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC),
				Result:    "audit finished but parser failed\nmanual note: suspicious workflow bypass",
			},
		},
	}

	report, err := Build(task, time.Date(2026, 3, 30, 12, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(report.Stages) != 1 {
		t.Fatalf("expected 1 stage, got %d", len(report.Stages))
	}
	if !report.Stages[0].RawOnly {
		t.Fatalf("expected raw-only stage fallback")
	}
	if !strings.Contains(report.Stages[0].RawResult, "workflow bypass") {
		t.Fatalf("expected raw result to be preserved, got %q", report.Stages[0].RawResult)
	}
}

func TestGenerateHTMLContainsIncludedStages(t *testing.T) {
	task := model.Task{
		ID:        "taskhtml12345",
		Name:      "HTML Audit",
		CreatedAt: time.Date(2026, 3, 30, 9, 0, 0, 0, time.UTC),
		Stages: []model.TaskStage{
			{
				Name:       "rce",
				Status:     "completed",
				UpdatedAt:  time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC),
				OutputJSON: json.RawMessage(`[{"type":"RCE","subtype":"Command Injection","severity":"CRITICAL","description":"Unsanitized shell exec","location":{"file":"cmd/app.go","line":88},"trigger":{"method":"POST","path":"/api/run","parameter":"cmd"}}]`),
			},
			{
				Name:       "auth",
				Status:     "completed",
				UpdatedAt:  time.Date(2026, 3, 30, 10, 10, 0, 0, time.UTC),
				OutputJSON: json.RawMessage(`[]`),
			},
		},
	}

	html, fileName, err := GenerateHTML(task, time.Date(2026, 3, 30, 13, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("GenerateHTML returned error: %v", err)
	}

	content := string(html)
	if !strings.Contains(content, "CodeScan 审计报告") {
		t.Fatalf("expected Chinese report header in HTML, got %q", content)
	}
	if !strings.Contains(content, "RCE 审计") || !strings.Contains(content, "认证与会话审计") {
		t.Fatalf("expected included stage labels in HTML, got %q", content)
	}
	if strings.Contains(content, "XSS Audit") {
		t.Fatalf("did not expect incomplete stages to appear in HTML")
	}
	if !strings.Contains(content, "该阶段已执行完成，未发现已确认漏洞。本结果会保留在导出报告中，便于后续追踪与留档。") {
		t.Fatalf("expected zero-finding section note in HTML")
	}
	if !strings.HasSuffix(fileName, ".html") {
		t.Fatalf("expected html filename, got %q", fileName)
	}
}
