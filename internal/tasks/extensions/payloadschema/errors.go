package payloadschema

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// ErrInvalidPayload is matched by every error reporting a submission that
// doesn't match its form, so the task handler can answer it with 400.
// errors.Is still matches after the orchestrator wraps it with %w.
var ErrInvalidPayload = errors.New("payload schema: invalid payload")

// Violation is one way a submission fails its form: Path is a JSON Pointer
// into the submitted payload, "" for the payload as a whole. Violations are
// logged, not returned to the caller.
type Violation struct {
	Path    string
	Message string
}

// InvalidPayloadError reports a submission that doesn't match its form. It
// matches ErrInvalidPayload.
type InvalidPayloadError struct {
	Violations []Violation
}

func (e *InvalidPayloadError) Error() string {
	parts := make([]string, len(e.Violations))
	for i, v := range e.Violations {
		parts[i] = v.Path + ": " + v.Message
	}
	return ErrInvalidPayload.Error() + ": " + strings.Join(parts, "; ")
}

// Is makes errors.Is(err, ErrInvalidPayload) true.
func (e *InvalidPayloadError) Is(target error) bool {
	return target == ErrInvalidPayload
}

var printer = message.NewPrinter(language.English)

// violations flattens the library's tree of errors into the failures a caller
// can act on: one per missing property, at the property, and one per failing
// rule elsewhere. A failed oneOf or anyOf is one failure at its value, not one
// per alternative.
func violations(err *jsonschema.ValidationError) []Violation {
	var out []Violation
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		switch k := e.ErrorKind.(type) {
		case *kind.Required:
			for _, name := range k.Missing {
				out = append(out, Violation{Path: pointer(append(slices.Clone(e.InstanceLocation), name)), Message: "missing property"})
			}
			return
		case *kind.OneOf, *kind.AnyOf:
			out = append(out, Violation{Path: pointer(e.InstanceLocation), Message: alternativesMessage(e)})
			return
		}
		if len(e.Causes) == 0 {
			out = append(out, Violation{Path: pointer(e.InstanceLocation), Message: e.ErrorKind.LocalizedString(printer)})
			return
		}
		for _, cause := range e.Causes {
			walk(cause)
		}
	}
	walk(err)
	return out
}

// alternativesMessage describes a failed oneOf or anyOf. When every
// alternative is a const, as the forms write their choice lists, it names the
// allowed values.
func alternativesMessage(e *jsonschema.ValidationError) string {
	allowed := make([]string, 0, len(e.Causes))
	for _, cause := range e.Causes {
		c, ok := cause.ErrorKind.(*kind.Const)
		if !ok || len(cause.Causes) > 0 {
			return e.ErrorKind.LocalizedString(printer)
		}
		allowed = append(allowed, fmt.Sprintf("%q", fmt.Sprint(c.Want)))
	}
	if len(allowed) == 0 {
		return e.ErrorKind.LocalizedString(printer)
	}
	return "value must be one of " + strings.Join(allowed, ", ")
}

// pointer returns the JSON Pointer (RFC 6901) for location.
func pointer(location []string) string {
	escape := strings.NewReplacer("~", "~0", "/", "~1")
	var b strings.Builder
	for _, token := range location {
		b.WriteString("/")
		b.WriteString(escape.Replace(token))
	}
	return b.String()
}
