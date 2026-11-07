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
