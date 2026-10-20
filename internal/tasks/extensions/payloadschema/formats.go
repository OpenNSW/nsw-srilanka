package payloadschema

import (
	"errors"
	"regexp"
	"strconv"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// ajvFormats are the formats on which this library and the trader app's
// ajv-formats (2.1.1, "full" mode) disagree, defined as ajv-formats defines
// them, so the server accepts exactly what the browser does. Formats the two
// agree on, such as "date", keep this library's definition.
var ajvFormats = []*jsonschema.Format{
	{Name: "time", Validate: validateTime},
	{Name: "email", Validate: validateEmail},
}

// ajvTime is ajv-formats' TIME: the timezone is optional, so "10:00:00", which
// the trader app sends, is valid. This library requires one.
var ajvTime = regexp.MustCompile(`(?i)^(\d\d):(\d\d):(\d\d)(\.\d+)?(z|[+-]\d\d(?::?\d\d)?)?$`)

// validateTime ports ajv-formats' time(str) with no timezone required: any
// hh:mm:ss up to 23:59:59, plus the leap second 23:59:60.
func validateTime(v any) error {
	s, ok := v.(string)
	if !ok {
		return nil
	}
	m := ajvTime.FindStringSubmatch(s)
	if m == nil {
		return errors.New("not a valid time")
	}
	hour, _ := strconv.Atoi(m[1])
	minute, _ := strconv.Atoi(m[2])
	second, _ := strconv.Atoi(m[3])
	if (hour <= 23 && minute <= 59 && second <= 59) || (hour == 23 && minute == 59 && second == 60) {
		return nil
	}
	return errors.New("not a valid time")
}

// ajvEmail is ajv-formats' full-mode email: the domain needs at least one dot,
// so "a@localhost" is invalid. This library accepts it.
var ajvEmail = regexp.MustCompile(`(?i)^[a-z0-9!#$%&'*+/=?^_` + "`" + `{|}~-]+(?:\.[a-z0-9!#$%&'*+/=?^_` + "`" + `{|}~-]+)*@(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

// validateEmail ports ajv-formats' full-mode email check.
func validateEmail(v any) error {
	s, ok := v.(string)
	if !ok {
		return nil
	}
	if !ajvEmail.MatchString(s) {
		return errors.New("not a valid email")
	}
	return nil
}
