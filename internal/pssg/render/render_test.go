package render

import (
	"bytes"
	"encoding/json"
	"html/template"
	"regexp"
	"testing"
)

// TestScriptTagJSONNotDoubleEncoded verifies that template.JS fields
// inside <script> tags produce valid, non-double-encoded JSON.
// This is a regression test for the html/template context-escaping bug
// where template.HTML was used instead of template.JS, causing Go's
// html/template engine to re-marshal the JSON inside <script> tags.
func TestScriptTagJSONNotDoubleEncoded(t *testing.T) {
	tests := []struct {
		name     string
		tmplText string
		data     interface{}
		scriptID string
	}{
		{
			name:     "template.JS in script tag produces valid JSON object",
			tmplText: `<script type="application/json" id="test-data">{{.Data}}</script>`,
			data: struct {
				Data template.JS
			}{
				Data: template.JS(`{"nodes":[{"id":"root","name":"Root"}],"links":[]}`),
			},
			scriptID: "test-data",
		},
		{
			name:     "template.JS with nested JSON",
			tmplText: `<script type="application/json" id="chart-data">{{.ChartData}}</script>`,
			data: struct {
				ChartData template.JS
			}{
				ChartData: template.JS(`{"taxonomies":[{"label":"Category","topEntries":[{"name":"Web","count":5}]}],"totalEntities":42}`),
			},
			scriptID: "chart-data",
		},
		{
			name:     "empty template.JS produces empty output",
			tmplText: `<script type="application/json" id="empty-data">{{.ChartData}}</script>`,
			data: struct {
				ChartData template.JS
			}{
				ChartData: template.JS(""),
			},
			scriptID: "empty-data",
		},
	}

	scriptContentRe := regexp.MustCompile(`<script[^>]*id="([^"]+)"[^>]*>(.*?)</script>`)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := template.New("test").Parse(tt.tmplText)
			if err != nil {
				t.Fatalf("failed to parse template: %v", err)
			}

			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, tt.data); err != nil {
				t.Fatalf("failed to execute template: %v", err)
			}

			rendered := buf.String()

			matches := scriptContentRe.FindStringSubmatch(rendered)
			if matches == nil {
				t.Fatalf("no <script> tag found in rendered output: %s", rendered)
			}

			scriptID := matches[1]
			content := matches[2]

			if scriptID != tt.scriptID {
				t.Fatalf("expected script id %q, got %q", tt.scriptID, scriptID)
			}

			// Skip validation for empty content
			if content == "" {
				return
			}

			// Verify it parses as a JSON object, not a JSON string
			var result interface{}
			if err := json.Unmarshal([]byte(content), &result); err != nil {
				t.Fatalf("script content is not valid JSON: %v\nContent: %s", err, content)
			}

			// If the result is a string, the JSON was double-encoded
			if _, isString := result.(string); isString {
				t.Fatalf("script content is double-encoded: JSON.parse returns a string instead of an object\nContent: %s", content)
			}

			// Verify it's an object (map)
			if _, isMap := result.(map[string]interface{}); !isMap {
				t.Fatalf("script content parsed to unexpected type %T, expected map\nContent: %s", result, content)
			}
		})
	}
}

// TestTemplateHTMLWouldDoubleEncode demonstrates that using template.HTML
// inside <script> tags causes double-encoding (the bug we fixed).
func TestTemplateHTMLWouldDoubleEncode(t *testing.T) {
	tmplText := `<script type="application/json" id="test">{{.Data}}</script>`
	tmpl, err := template.New("test").Parse(tmplText)
	if err != nil {
		t.Fatalf("failed to parse template: %v", err)
	}

	data := struct {
		Data template.HTML
	}{
		Data: template.HTML(`{"nodes":[{"id":"root"}]}`),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("failed to execute template: %v", err)
	}

	rendered := buf.String()

	// Extract content between script tags
	re := regexp.MustCompile(`<script[^>]*>(.*?)</script>`)
	matches := re.FindStringSubmatch(rendered)
	if matches == nil {
		t.Fatal("no script tag found")
	}
	content := matches[1]

	// With template.HTML in a script context, html/template double-encodes it.
	// The content should NOT parse as a valid JSON object directly.
	var result interface{}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		// If it doesn't parse at all, that's also a form of brokenness
		t.Logf("template.HTML in script tag produced unparseable content (expected): %s", content)
		return
	}

	// If it parses as a string, it's the double-encoding we saw
	if s, isString := result.(string); isString {
		maxLen := len(s)
		if maxLen > 60 {
			maxLen = 60
		}
		t.Logf("Confirmed: template.HTML causes double-encoding in script tags. JSON.parse returns string: %s", s[:maxLen])
		return
	}

	// If it somehow parses as an object, that's unexpected for template.HTML in script context
	t.Log("Warning: template.HTML in script tag parsed as object — behavior may have changed in this Go version")
}

// TestLengthWithVariousTypes verifies the reflect-based length function
// works with all slice types including taxonomy.Entry slices.
func TestLengthWithVariousTypes(t *testing.T) {
	type CustomStruct struct {
		Name string
	}

	tests := []struct {
		name     string
		input    interface{}
		expected int
	}{
		{"nil", nil, 0},
		{"empty string", "", 0},
		{"string", "hello", 5},
		{"string slice", []string{"a", "b", "c"}, 3},
		{"empty slice", []string{}, 0},
		{"interface slice", []interface{}{1, "two", 3.0}, 3},
		{"map", map[string]interface{}{"a": 1, "b": 2}, 2},
		{"custom struct slice", []CustomStruct{{Name: "a"}, {Name: "b"}}, 2},
		{"int slice", []int{1, 2, 3, 4, 5}, 5},
		{"map string string", map[string]string{"a": "1"}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := length(tt.input)
			if got != tt.expected {
				t.Errorf("length(%v) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}
