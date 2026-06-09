package model

import "encoding/json"

// StringArray stores identifier lists as plain strings.
type StringArray []string

// MarshalJSON serializes StringArray as a standard JSON string array.
func (a StringArray) MarshalJSON() ([]byte, error) {
	if a == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]string(a))
}

// UnmarshalJSON deserializes StringArray without UUID format validation.
func (a *StringArray) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*a = []string{}
		return nil
	}

	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		*a = []string{}
		return nil
	}

	*a = ids
	return nil
}
