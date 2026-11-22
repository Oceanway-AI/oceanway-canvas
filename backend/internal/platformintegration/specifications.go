package platformintegration

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type publishedSchemaLoader struct{}

func (publishedSchemaLoader) Load(string) (any, error) {
	return nil, errors.New("external schema references are not supported")
}

// Apply the whole selected schema, including conditional requirements. Never
// fetch remote documents or files while compiling a published schema.
func ValidateSpecifications(raw json.RawMessage, value map[string]any) error {
	schema, err := compileSpecifications(raw)
	if err != nil {
		return err
	}
	if schema.Validate(value) != nil {
		return errors.New("生成参数不符合所选模型的发布规格")
	}
	return nil
}

func compileSpecifications(raw json.RawMessage) (*jsonschema.Schema, error) {
	var doc any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&doc) != nil {
		return nil, errors.New("模型参数合同无效")
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(publishedSchemaLoader{})
	if compiler.AddResource("https://studio.invalid/published-schema.json", doc) != nil {
		return nil, errors.New("模型参数合同无效")
	}
	schema, err := compiler.Compile("https://studio.invalid/published-schema.json")
	if err != nil {
		return nil, errors.New("模型参数合同暂不受支持")
	}
	return schema, nil
}

// The catalog knows the host's material envelope but not the future prompt
// (represented by nil) or the settings. Only reject a provable contradiction.
func MaterialSchemaCompatible(raw json.RawMessage, envelope map[string]any, slots []NativeReferenceSlot) bool {
	if len(slots) == 0 {
		return true
	}
	schema, err := compileSpecifications(raw)
	if err != nil {
		return false
	}
	var matches func(*jsonschema.Schema, any, string, bool) bool
	matches = func(s *jsonschema.Schema, value any, path string, fixed bool) bool {
		if fixed && !containsUnknownPrompt(value) {
			return s.Validate(value) == nil
		}
		testBranch := func(branch *jsonschema.Schema) bool { return matches(branch, value, path, fixed) }
		for _, sub := range s.AllOf {
			if !testBranch(sub) {
				return false
			}
		}
		for _, branches := range [][]*jsonschema.Schema{s.AnyOf, s.OneOf} {
			if len(branches) > 0 && !slices.ContainsFunc(branches, testBranch) {
				return false
			}
		}
		switch v := value.(type) {
		case map[string]any:
			for _, key := range s.Required {
				child := path + "/" + escapePointerPart(key)
				if _, present := v[key]; !present && (fixed || slices.ContainsFunc(slots, func(slot NativeReferenceSlot) bool { return slot.Pointer == child })) {
					return false
				}
			}
			for key, sub := range s.Properties {
				child := path + "/" + escapePointerPart(key)
				exact := fixed || slices.ContainsFunc(slots, func(slot NativeReferenceSlot) bool { return slot.Pointer == child })
				if !exact && !slices.ContainsFunc(slots, func(slot NativeReferenceSlot) bool { return strings.HasPrefix(slot.Pointer, child+"/") }) {
					continue
				}
				item, present := v[key]
				if !present {
					if !slices.Contains(s.Required, key) {
						continue
					}
					// A caller setting may create this object, but it cannot
					// create a missing material inside the required object.
					item = map[string]any{}
				}
				if !matches(sub, item, child, exact) {
					return false
				}
			}
		case []any:
			if !fixed {
				return true
			}
			if s.MinItems != nil && len(v) < *s.MinItems || s.MaxItems != nil && len(v) > *s.MaxItems {
				return false
			}
			if s.Contains != nil {
				minimum, possible := 1, 0
				if s.MinContains != nil {
					minimum = *s.MinContains
				}
				for _, item := range v {
					if matches(s.Contains, item, path, true) {
						possible++
					}
				}
				return possible >= minimum
			}
		}
		return true
	}
	return matches(schema, envelope, "", false)
}

func escapePointerPart(key string) string {
	return strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}

func containsUnknownPrompt(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case []any:
		return slices.ContainsFunc(v, containsUnknownPrompt)
	case map[string]any:
		for _, child := range v {
			if containsUnknownPrompt(child) {
				return true
			}
		}
	}
	return false
}
