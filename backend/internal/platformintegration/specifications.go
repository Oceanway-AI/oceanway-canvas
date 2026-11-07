package platformintegration

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type publishedSchemaLoader struct{}

func (publishedSchemaLoader) Load(string) (any, error) {
	return nil, errors.New("external schema references are not supported")
}

// Apply the whole selected schema, including conditional requirements. Never
// fetch remote documents or files while compiling a published schema.
func ValidateSpecifications(raw json.RawMessage, value map[string]any) error {
	var doc any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&doc) != nil {
		return errors.New("模型参数合同无效")
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(publishedSchemaLoader{})
	if compiler.AddResource("https://studio.invalid/published-schema.json", doc) != nil {
		return errors.New("模型参数合同无效")
	}
	schema, err := compiler.Compile("https://studio.invalid/published-schema.json")
	if err != nil {
		return errors.New("模型参数合同暂不受支持")
	}
	if schema.Validate(value) != nil {
		return errors.New("生成参数不符合所选模型的发布规格")
	}
	return nil
}
