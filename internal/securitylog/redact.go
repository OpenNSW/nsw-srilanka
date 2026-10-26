package securitylog

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

const redactedValue = "[redacted]"

// sensitiveKey reports names whose values grant access or are a credential.
// Payment amounts, form fields, and contact details are not in this set: the
// security line exists to record those payloads.
func sensitiveKey(name string) bool {
	switch strings.ToLower(name) {
	case "upload_url", "download_url",
		"token", "callbacktoken", "access_token", "refresh_token", "id_token",
		"password", "secret", "api_key", "apikey",
		"authorization", "signature", "private_key", "client_secret":
		return true
	default:
		return false
	}
}

func presignedURL(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "x-amz-signature=") ||
		strings.Contains(lower, "x-amz-credential=") ||
		strings.Contains(lower, "x-amz-security-token=")
}

// redactLogged removes credential values from a text body. A valid JSON value
// is rewritten in place. A truncated JSON body, which does not parse, is
// scrubbed as text so a signed URL cut off at the cap is still removed.
func redactLogged(contentType string, body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	ct := mediaType(contentType)
	switch {
	case ct == "application/x-www-form-urlencoded":
		return redactForm(body)
	case ct == "application/json" || strings.HasSuffix(ct, "+json") || ct == "":
		if out, ok := redactJSON(body); ok {
			return out
		}
		return redactJSONText(body)
	default:
		return body
	}
}

func mediaType(contentType string) string {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	return ct
}

func redactJSON(body []byte) ([]byte, bool) {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, false
	}
	if !redactAny(v) {
		return body, true
	}
	out, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	return out, true
}

func redactAny(v any) bool {
	switch t := v.(type) {
	case map[string]any:
		changed := false
		for k, val := range t {
			if sensitiveKey(k) {
				t[k] = redactedValue
				changed = true
				continue
			}
			if s, ok := val.(string); ok && presignedURL(s) {
				t[k] = redactedValue
				changed = true
				continue
			}
			if redactAny(val) {
				changed = true
			}
		}
		return changed
	case []any:
		changed := false
		for _, item := range t {
			if redactAny(item) {
				changed = true
			}
		}
		return changed
	default:
		return false
	}
}

// jsonStringValue matches a JSON string field with a sensitive name, including
// a value that was cut off before its closing quote.
var jsonStringValue = regexp.MustCompile(`(?i)"(upload_url|download_url|access_token|refresh_token|id_token|private_key|client_secret|api_key|apikey|authorization|password|signature|callbacktoken|token|secret)"\s*:\s*"(?:\\.|[^"\\]*)"?`)

// presignedString matches a JSON string that carries an S3 presigned query.
var presignedString = regexp.MustCompile(`"(?:\\.|[^"\\])*(?:X-Amz-Signature|X-Amz-Credential|X-Amz-Security-Token)=(?:\\.|[^"\\]*)"?`)

func redactJSONText(body []byte) []byte {
	s := jsonStringValue.ReplaceAllString(string(body), `"$1":"`+redactedValue+`"`)
	s = presignedString.ReplaceAllString(s, `"`+redactedValue+`"`)
	return []byte(s)
}

func redactForm(body []byte) []byte {
	v, err := url.ParseQuery(string(body))
	if err != nil {
		return body
	}
	changed := false
	for k := range v {
		if sensitiveKey(k) {
			v[k] = []string{redactedValue}
			changed = true
		}
	}
	if !changed {
		return body
	}
	return []byte(v.Encode())
}

// loggedQuery redacts credential parameters and leaves every other query as it
// arrived, so an ordinary query is not re-encoded.
func loggedQuery(raw string) string {
	if raw == "" {
		return ""
	}
	v, err := url.ParseQuery(raw)
	if err != nil {
		return raw
	}
	changed := false
	for k := range v {
		if sensitiveKey(k) {
			v[k] = []string{redactedValue}
			changed = true
		}
	}
	if !changed {
		return raw
	}
	return v.Encode()
}
