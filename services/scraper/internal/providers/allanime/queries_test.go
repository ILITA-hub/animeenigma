package allanime

import (
	"strings"
	"testing"
)

func TestBuildSourcesVariables_TranslationType(t *testing.T) {
	for _, tt := range []string{"sub", "dub", "raw"} {
		got, err := buildSourcesVariables("SHOW123", "5", tt)
		if err != nil {
			t.Fatalf("buildSourcesVariables(%q): %v", tt, err)
		}
		if !strings.Contains(got, `"translationType":"`+tt+`"`) {
			t.Errorf("translationType %q not in vars: %s", tt, got)
		}
	}
	got, _ := buildSourcesVariables("SHOW123", "5", "")
	if !strings.Contains(got, `"translationType":"sub"`) {
		t.Errorf("empty type should default to sub: %s", got)
	}
}
