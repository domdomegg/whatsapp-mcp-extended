package mcp

import (
	"encoding/json"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestJSONResultStructuredContentOnlyForObjects(t *testing.T) {
	obj := jsonResult(m{"success": true})
	if obj.StructuredContent == nil || obj.IsError {
		t.Errorf("object result should carry structured content: %+v", obj)
	}
	for name, v := range map[string]any{"array": []int{1, 2}, "empty array": []string{}, "null": nil, "string": "x"} {
		r := jsonResult(v)
		if r.StructuredContent != nil {
			t.Errorf("%s: structuredContent must be an object or absent, got %v", name, r.StructuredContent)
		}
		text := r.Content[0].(*sdk.TextContent).Text
		if !json.Valid([]byte(text)) {
			t.Errorf("%s: text content is not JSON: %s", name, text)
		}
	}
}
