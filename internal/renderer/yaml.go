package renderer

import (
	"gopkg.in/yaml.v3"
)

// parseYAMLSecrets parses a YAML file into a map[string]string.
// All values must be strings — this is for flat secret files, not nested configs.
func parseYAMLSecrets(data []byte) (map[string]string, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	result := make(map[string]string, len(raw))
	for k, v := range raw {
		switch val := v.(type) {
		case string:
			result[k] = val
		case int:
			result[k] = string(rune(val))
		case bool:
			if val {
				result[k] = "true"
			} else {
				result[k] = "false"
			}
		default:
			// For any other type, skip — secrets should be strings
			continue
		}
	}
	return result, nil
}
