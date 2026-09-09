package main

import (
	"strings"
	"testing"
)

func TestParseApplyFile(t *testing.T) {
	t.Run("missing companies key", func(t *testing.T) {
		_, err := parseApplyFile([]byte(`{}`))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), `"companies"`) {
			t.Errorf("expected error mentioning companies, got: %v", err)
		}
	})

	t.Run("null companies value", func(t *testing.T) {
		_, err := parseApplyFile([]byte(`{"companies": null}`))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), `"companies"`) {
			t.Errorf("expected error mentioning companies, got: %v", err)
		}
	})

	t.Run("top-level null", func(t *testing.T) {
		_, err := parseApplyFile([]byte(`null`))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("explicit empty array is allowed", func(t *testing.T) {
		specs, err := parseApplyFile([]byte(`{"companies": []}`))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if specs == nil {
			t.Fatal("expected empty slice, got nil")
		}
		if len(specs) != 0 {
			t.Errorf("expected 0 specs, got %d", len(specs))
		}
	})

	t.Run("valid companies", func(t *testing.T) {
		specs, err := parseApplyFile([]byte(`{"companies": [{"id":"co-1","name":"ACME"}]}`))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(specs) != 1 {
			t.Fatalf("expected 1 spec, got %d", len(specs))
		}
		if specs[0].ID != "co-1" || specs[0].Name != "ACME" {
			t.Errorf("got %+v, want id=co-1 name=ACME", specs[0])
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		_, err := parseApplyFile([]byte(`{`))
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
