package dispatcher

import (
	"encoding/json"
	"testing"
)

// TestAnalysisSchemaValid asserts analysisSchema is well-formed JSON and that
// the issue.status property carries an `enum` array constraining the LLM to a
// known set of statuses — including the dedicated "captured" capture token —
// rather than free text. This locks the schema hardening for finding L585.
func TestAnalysisSchemaValid(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal([]byte(analysisSchema), &schema); err != nil {
		t.Fatalf("analysisSchema is not valid JSON: %v", err)
	}

	statusEnum := issueStatusEnum(t, schema)
	if len(statusEnum) == 0 {
		t.Fatalf("issue.status property has no enum constraint")
	}

	want := []string{"captured", "open", "resolved", "auto_fixed", "backlog", "todo"}
	for _, w := range want {
		if !contains(statusEnum, w) {
			t.Fatalf("issue.status enum is missing %q; got %v", w, statusEnum)
		}
	}
}

// issueStatusEnum walks schema.properties.issue.properties.status.enum.
func issueStatusEnum(t *testing.T, schema map[string]any) []string {
	t.Helper()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema has no properties object")
	}
	issue, ok := props["issue"].(map[string]any)
	if !ok {
		t.Fatalf("schema.properties has no issue object")
	}
	issueProps, ok := issue["properties"].(map[string]any)
	if !ok {
		t.Fatalf("issue has no properties object")
	}
	status, ok := issueProps["status"].(map[string]any)
	if !ok {
		t.Fatalf("issue.properties has no status object")
	}
	raw, ok := status["enum"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func contains(ss []string, target string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}
