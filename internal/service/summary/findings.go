package summary

import (
	"encoding/json"
	"strings"
)

const (
	VerificationStatusUnreviewed = "unreviewed"
	VerificationStatusConfirmed  = "confirmed"
	VerificationStatusUncertain  = "uncertain"
	VerificationStatusRejected   = "rejected"
)

func ParseJSONArray(raw json.RawMessage, fallback string) ([]map[string]any, bool) {
	findings, _, ok := ParseFindings(raw, fallback)
	if !ok || findings == nil {
		return nil, false
	}
	return findings, true
}

func FindingVerificationStatus(finding map[string]any) string {
	status := strings.ToLower(strings.TrimSpace(ExtractString(finding["verification_status"])))
	switch status {
	case VerificationStatusConfirmed, VerificationStatusUncertain, VerificationStatusRejected:
		return status
	default:
		return VerificationStatusUnreviewed
	}
}

func IsRejectedFinding(finding map[string]any) bool {
	return FindingVerificationStatus(finding) == VerificationStatusRejected
}

func EffectiveSeverity(finding map[string]any) string {
	if reviewed := strings.TrimSpace(ExtractString(finding["reviewed_severity"])); reviewed != "" {
		return NormalizeSeverity(reviewed)
	}
	return NormalizeSeverity(ExtractString(finding["severity"]))
}

func ActiveFindings(findings []map[string]any) []map[string]any {
	active := make([]map[string]any, 0, len(findings))
	for _, finding := range findings {
		if IsRejectedFinding(finding) {
			continue
		}
		active = append(active, finding)
	}
	return active
}

func RejectedFindings(findings []map[string]any) []map[string]any {
	rejected := make([]map[string]any, 0)
	for _, finding := range findings {
		if IsRejectedFinding(finding) {
			rejected = append(rejected, finding)
		}
	}
	return rejected
}
