package run

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-rod/rod"
)

type Report struct {
	RunID      string            `json:"run_id"`
	FlowID     string            `json:"flow_id"`
	Status     string            `json:"status"` // pass|fail
	TargetURL  string            `json:"target_url"`
	StartedAt  string            `json:"started_at"`
	DurationMS int64             `json:"duration_ms"`
	Steps      []StepResult      `json:"steps,omitempty"`
	Assertions []AssertionResult `json:"assertions,omitempty"`
	Artifacts  Artifacts         `json:"artifacts,omitempty"`
	Error      string            `json:"error,omitempty"`
}

type StepResult struct {
	Index   int    `json:"index"`
	Action  string `json:"action"`
	Target  string `json:"target,omitempty"`
	Status  string `json:"status"` // ok|fail
	Error   string `json:"error,omitempty"`
	Elapsed int64  `json:"elapsed_ms,omitempty"`
}

type AssertionResult struct {
	Index   int    `json:"index"`
	Type    string `json:"type"`
	Target  string `json:"target,omitempty"`
	Value   string `json:"value,omitempty"`
	Status  string `json:"status"` // ok|fail
	Error   string `json:"error,omitempty"`
	Elapsed int64  `json:"elapsed_ms,omitempty"`
}

type Artifacts struct {
	ReportPath    string `json:"report_path,omitempty"`
	ScreenshotPNG string `json:"screenshot_png,omitempty"`
	DOMHTML       string `json:"dom_html,omitempty"`
}

func NewRunID(now time.Time) string {
	return now.UTC().Format("2006-01-02T15-04-05Z")
}

func EnsureRunDirs(outputRoot string, runID string, flowID string) (runDir string, flowDir string, err error) {
	if outputRoot == "" {
		return "", "", fmt.Errorf("outputRoot is empty")
	}
	if runID == "" {
		return "", "", fmt.Errorf("runID is empty")
	}
	if flowID == "" {
		return "", "", fmt.Errorf("flowID is empty")
	}

	runDir = filepath.Join(outputRoot, runID)
	flowDir = filepath.Join(runDir, "flows", flowID)
	if err := os.MkdirAll(flowDir, 0o755); err != nil {
		return "", "", err
	}
	return runDir, flowDir, nil
}

func WriteReportJSON(reportPath string, r Report) error {
	if reportPath == "" {
		return fmt.Errorf("reportPath is empty")
	}
	r.Artifacts.ReportPath = reportPath
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := reportPath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, reportPath)
}

func CaptureFailureArtifacts(flowDir string, page *rod.Page) (screenshotPath string, domPath string, err error) {
	if flowDir == "" {
		return "", "", fmt.Errorf("flowDir is empty")
	}
	if page == nil {
		return "", "", fmt.Errorf("page is nil")
	}

	screenshotPath = filepath.Join(flowDir, "snapshot.png")
	page.MustScreenshot(screenshotPath)

	domPath = filepath.Join(flowDir, "dom.html")
	dom := page.MustEval(`() => document.documentElement.outerHTML`).String()
	if err := os.WriteFile(domPath, []byte(dom), 0o644); err != nil {
		return screenshotPath, "", err
	}
	return screenshotPath, domPath, nil
}
