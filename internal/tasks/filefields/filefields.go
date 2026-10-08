// Package filefields reads which values in a task's data are stored files, as
// the task's render config declares them, and rewrites those values.
//
// A render config lists its file fields at the top level:
//
//	"files": ["traderinput.invoice", "userform.documents[*].file"]
//
// Each entry is a path into the task's data:
//
//	path    = segment { "." segment }
//	segment = name { "[*]" }
//	name    = 1*( letter | digit | "_" | "-" )
//
// A name steps into an object's field, and each "[*]" steps into every element
// of an array, so arrays nest to any depth.
//
// The package is pure: it does no I/O and knows nothing of what the values
// are rewritten to.
package filefields

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Path is one declared file field.
type Path struct {
	segments []segment
}

// segment is a name followed by the number of "[*]" after it.
type segment struct {
	name   string
	arrays int
}

var namePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Parse returns the file fields renderConfig declares, in the order it lists
// them. A render config without "files" declares none. A path that does not
// parse is an error, not skipped: a file field left out would go unprotected.
func Parse(renderConfig json.RawMessage) ([]Path, error) {
	if len(renderConfig) == 0 {
		return nil, nil
	}
	var decl struct {
		Files []string `json:"files"`
	}
	if err := json.Unmarshal(renderConfig, &decl); err != nil {
		return nil, fmt.Errorf("filefields: decode files: %w", err)
	}
	paths := make([]Path, 0, len(decl.Files))
	for _, s := range decl.Files {
		p, err := parsePath(s)
		if err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, nil
}

func parsePath(s string) (Path, error) {
	parts := strings.Split(s, ".")
	segments := make([]segment, 0, len(parts))
	for _, part := range parts {
		name, arrays := part, 0
		for strings.HasSuffix(name, "[*]") {
			name, arrays = strings.TrimSuffix(name, "[*]"), arrays+1
		}
		if !namePattern.MatchString(name) {
			return Path{}, fmt.Errorf("filefields: file field %q: %q is not a name followed by any number of [*]", s, part)
		}
		segments = append(segments, segment{name: name, arrays: arrays})
	}
	return Path{segments: segments}, nil
}

// String returns p as it is written in a render config.
func (p Path) String() string {
	parts := make([]string, len(p.segments))
	for i, seg := range p.segments {
		parts[i] = seg.name + strings.Repeat("[*]", seg.arrays)
	}
	return strings.Join(parts, ".")
}

// Rebase returns p relative to the value at the top-level field prefix, for
// data that holds only that value: "traderinput.invoice" rebased on
// "traderinput" is "invoice". ok is false when p does not lie inside prefix.
// An empty prefix returns p unchanged.
func (p Path) Rebase(prefix string) (rebased Path, ok bool) {
	if prefix == "" {
		return p, true
	}
	if len(p.segments) < 2 || p.segments[0] != (segment{name: prefix}) {
		return Path{}, false
	}
	return Path{segments: p.segments[1:]}, true
}

// Replace replaces each non-empty string at paths in data with fn's result,
// in place, so the caller passes its own copy. data must be JSON-decoded
// (objects as map[string]any, arrays as []any). A path that is missing, or
// meets a value of another shape, is skipped, as is an empty string or any
// other value at the end of a path: there is no file there. The first error
// from fn stops the walk and is returned wrapped, so errors.Is sees it.
func Replace(data any, paths []Path, fn func(string) (string, error)) error {
	for _, p := range paths {
		if err := replaceIn(data, p.segments, fn); err != nil {
			return fmt.Errorf("filefields: %s: %w", p, err)
		}
	}
	return nil
}

// replaceIn replaces the strings at segments under node, an object.
func replaceIn(node any, segments []segment, fn func(string) (string, error)) error {
	obj, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	seg := segments[0]
	v, ok := obj[seg.name]
	if !ok {
		return nil
	}
	replaced, err := replaceValue(v, seg.arrays, segments[1:], fn)
	if err != nil {
		return err
	}
	obj[seg.name] = replaced
	return nil
}

// replaceValue returns v with the strings under it replaced: it steps into
// arrays levels of array, then follows rest.
func replaceValue(v any, arrays int, rest []segment, fn func(string) (string, error)) (any, error) {
	if arrays > 0 {
		arr, ok := v.([]any)
		if !ok {
			return v, nil
		}
		for i, el := range arr {
			replaced, err := replaceValue(el, arrays-1, rest, fn)
			if err != nil {
				return nil, err
			}
			arr[i] = replaced
		}
		return arr, nil
	}
	if len(rest) > 0 {
		return v, replaceIn(v, rest, fn)
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return v, nil
	}
	return fn(s)
}
