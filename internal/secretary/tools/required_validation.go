package tools

import (
	"fmt"
	"strings"
)

func validateRequired(fields map[string]string, required []string, anyOfRequired [][]string) error {
	if fields == nil {
		fields = map[string]string{}
	}

	required = trimStringList(required)
	anyOfRequired = trimStringGroups(anyOfRequired)

	var missing []string
	for _, key := range required {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if strings.TrimSpace(fields[key]) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) == 1 {
		if missing[0] == "confirm" {
			return fmt.Errorf("confirm=true is required")
		}
		return fmt.Errorf("missing required field: %s", missing[0])
	}
	if len(missing) > 1 {
		return fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}

	for _, group := range anyOfRequired {
		group = trimStringList(group)
		if len(group) == 0 {
			continue
		}
		satisfied := false
		for _, key := range group {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if strings.TrimSpace(fields[key]) != "" {
				satisfied = true
				break
			}
		}
		if !satisfied {
			return fmt.Errorf("missing one of required fields: %s", strings.Join(group, " | "))
		}
	}
	return nil
}
