// SPDX-License-Identifier: GPL-3.0-or-later
package protonnode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/leggerezza/android-proton/core/scan"
)

// checkRequired verifies that every field the schema marks as required is
// present in the payload, that state/level/stage stay inside their enums, and
// that no undocumented top-level field appears. It reads the schema file so the
// document and the code cannot drift apart.
func checkRequired(t *testing.T, schemaPath string, payload []byte) {
	t.Helper()
	raw, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Enum []string `json:"enum"`
			Type string   `json:"type"`
		} `json:"properties"`
		AdditionalProperties *bool `json:"additionalProperties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	for _, key := range schema.Required {
		if _, ok := got[key]; !ok {
			t.Fatalf("required field %q missing from %s", key, payload)
		}
	}
	if schema.AdditionalProperties != nil && !*schema.AdditionalProperties {
		for key := range got {
			if _, ok := schema.Properties[key]; !ok {
				t.Fatalf("undocumented field %q in %s", key, payload)
			}
		}
	}
	for key, prop := range schema.Properties {
		if len(prop.Enum) == 0 {
			continue
		}
		value, ok := got[key].(string)
		if !ok {
			continue
		}
		found := false
		for _, want := range prop.Enum {
			if value == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("field %q = %q is outside its enum %v", key, value, prop.Enum)
		}
	}
}

func schemaPath(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "docs", "schemas", name))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("schema not found at %s: %v", p, err)
	}
	return p
}

func TestScanPayloadsMatchSchema(t *testing.T) {
	runner, err := scan.New([]scan.Target{{Server: "X#1", EntryIP: "127.0.0.1", Port: 9}}, scan.Options{Level: scan.LevelTCP, L1Timeout: 200000000})
	if err != nil {
		t.Fatal(err)
	}
	progress := runner.ProgressJSON()
	checkRequired(t, schemaPath(t, "scan-progress.schema.json"), []byte(progress))

	results := runner.ResultsJSON("wifi-A", false)
	checkRequired(t, schemaPath(t, "scan-results.schema.json"), []byte(results))

	// The not-found payloads must stay schema-shaped too: the app polls a scan id
	// that may already be gone after a process restart.
	checkRequired(t, schemaPath(t, "scan-progress.schema.json"), []byte(ScanProgress("scan-dead")))
	checkRequired(t, schemaPath(t, "scan-results.schema.json"), []byte(ScanResults("scan-dead")))
}
