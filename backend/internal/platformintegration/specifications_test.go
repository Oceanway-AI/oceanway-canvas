package platformintegration

import (
	"encoding/json"
	"testing"
)

func TestPublishedSpecificationsValidateFullSelectedSchema(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"size":{"enum":["square"]},"n":{"type":"integer","minimum":1,"maximum":2},"seed":{"type":"integer"}},"required":["size","n"],"if":{"properties":{"n":{"const":2}}},"then":{"required":["seed"]}}`)
	if err := ValidateSpecifications(schema, map[string]any{"size": "square", "n": 1}); err != nil {
		t.Fatal(err)
	}
	for _, in := range []map[string]any{{"size": "square", "n": 2}, {"size": "wide", "n": 1}, {"size": "square", "n": 1, "url": "https://external.test"}} {
		if ValidateSpecifications(schema, in) == nil {
			t.Fatal("invalid specification admitted")
		}
	}
	if ValidateSpecifications(json.RawMessage(`{"$ref":"http://127.0.0.1/private"}`), map[string]any{}) == nil {
		t.Fatal("external schema admitted")
	}
}

func TestMaterialSchemaCompatibilityLeavesEditableInputsOpen(t *testing.T) {
	envelope := map[string]any{"content": []any{map[string]any{"type": "text", "text": nil}}}
	slots := []NativeReferenceSlot{{Pointer: "/content"}, {Pointer: "/metadata/images"}}
	for _, tc := range []struct {
		name, rule string
		accept     bool
	}{
		{"a future prompt may satisfy its own pattern", `{"properties":{"content":{"contains":{"required":["text"],"properties":{"text":{"pattern":"^scene"}}}}}}`, true},
		{"the prompt cannot supply a material role", `{"properties":{"content":{"contains":{"required":["role"]}}}}`, false},
		{"optional parent does not demand references", `{"properties":{"metadata":{"required":["images"]}}}`, true},
		{"required parent does demand references", `{"required":["metadata"],"properties":{"metadata":{"required":["images"]}}}`, false},
		{"writable alternative can still match", `{"oneOf":[{"required":["metadata"],"properties":{"metadata":{"required":["images"]}}},{"required":["duration"]}]}`, true},
		{"a material required only for an editable parameter is undecided", `{"if":{"properties":{"duration":{"const":12}}},"then":{"required":["metadata"],"properties":{"metadata":{"required":["images"]}}}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := MaterialSchemaCompatible(json.RawMessage(tc.rule), envelope, slots); got != tc.accept {
				t.Fatalf("compatibility = %v, want %v", got, tc.accept)
			}
		})
	}
}
