package protocol

import (
	"encoding/json"
	"fmt"
)

// Reject retired contributions even when a package also contains creative tools.
func (c *ManifestContributions) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range []string{"paymentProviders", "smsProviders"} {
		if _, present := fields[key]; present {
			return fmt.Errorf("Studio 仅支持创作插件，已退出贡献类型 %q", key)
		}
	}
	type contributions ManifestContributions
	return json.Unmarshal(data, (*contributions)(c))
}
