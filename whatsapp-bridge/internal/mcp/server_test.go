package mcp

import (
	"encoding/json"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestJSONResultAlwaysStructuredAndTextMirrorsIt(t *testing.T) {
	cases := map[string]struct {
		in   any
		want string
	}{
		"object passes through": {m{"success": true}, `{"success":true}`},
		"array is wrapped":      {[]int{1, 2}, `{"result":[1,2]}`},
		"empty array":           {[]string{}, `{"result":[]}`},
		"null is wrapped":       {nil, `{"result":null}`},
		"string is wrapped":     {"x", `{"result":"x"}`},
	}
	for name, c := range cases {
		r := jsonResult(c.in)
		if r.IsError || r.StructuredContent == nil {
			t.Errorf("%s: want structured content, got %+v", name, r)
			continue
		}
		got, _ := json.Marshal(r.StructuredContent)
		if string(got) != c.want {
			t.Errorf("%s: structured = %s, want %s", name, got, c.want)
		}
		text := r.Content[0].(*sdk.TextContent).Text
		var fromText, fromStructured any
		if json.Unmarshal([]byte(text), &fromText) != nil {
			t.Errorf("%s: text is not JSON: %s", name, text)
		}
		b, _ := json.Marshal(r.StructuredContent)
		_ = json.Unmarshal(b, &fromStructured)
		if a, b := mustJSON(fromText), mustJSON(fromStructured); a != b {
			t.Errorf("%s: text %s does not mirror structured %s", name, a, b)
		}
	}
}

func TestOutputSchemaFollowsShape(t *testing.T) {
	if s := readOnly().outputSchema(); s["type"] != "object" || s["properties"] != nil {
		t.Errorf("object schema: %v", s)
	}
	list := readOnly().list().outputSchema()
	props := list["properties"].(map[string]any)["result"].(map[string]any)
	if props["type"] != "array" {
		t.Errorf("list schema: %v", list)
	}
	nullable := readOnly().nullable().outputSchema()
	nprops := nullable["properties"].(map[string]any)["result"].(map[string]any)
	if ts, ok := nprops["type"].([]string); !ok || len(ts) != 2 {
		t.Errorf("nullable schema: %v", nullable)
	}
	if !readOnly().list().readOnly { // shape setters keep the other hints
		t.Error("list() dropped readOnly")
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
