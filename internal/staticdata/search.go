package staticdata

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/OpenNSW/core/pagination"
)

// Option is one selectable row in a static_data artifact.
type Option struct {
	Const string `json:"const"`
	Title string `json:"title"`
}

// SearchResult is the pagination envelope returned when a static-data request
// includes q, offset, or limit.
type SearchResult = pagination.Page[Option]

// Search loads nothing itself: raw is one static_data artifact body. It keeps
// object entries that have a non-empty const and title, ranks them against
// query, and returns one page. query is matched case-insensitively against
// title and const. An empty query keeps artifact order.
func Search(raw json.RawMessage, query string, offset, limit int) (SearchResult, error) {
	options, err := parseOptions(raw)
	if err != nil {
		return SearchResult{}, err
	}
	matched := rank(options, query)
	page := pageOptions(matched, offset, limit)
	return pagination.NewPageResult(page, int64(len(matched)), offset, limit), nil
}

func parseOptions(raw json.RawMessage) ([]Option, error) {
	var envelope struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("parse static data options: %w", err)
	}
	options := make([]Option, 0, len(envelope.Data))
	for _, item := range envelope.Data {
		var opt Option
		if err := json.Unmarshal(item, &opt); err != nil {
			continue
		}
		if opt.Const == "" || opt.Title == "" {
			continue
		}
		options = append(options, opt)
	}
	return options, nil
}

// rank scores a lowercased query in four tiers: exact title or const, prefix
// of title or const, a later word in the title, then a substring of either.
// Equal scores prefer the shorter title, then const, which is the unique key.
func rank(options []Option, query string) []Option {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return options
	}

	type scored struct {
		opt   Option
		score int
	}
	matched := make([]scored, 0, len(options))
	for _, opt := range options {
		score, ok := matchScore(opt, q)
		if !ok {
			continue
		}
		matched = append(matched, scored{opt: opt, score: score})
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].score != matched[j].score {
			return matched[i].score < matched[j].score
		}
		if len(matched[i].opt.Title) != len(matched[j].opt.Title) {
			return len(matched[i].opt.Title) < len(matched[j].opt.Title)
		}
		return strings.ToLower(matched[i].opt.Const) < strings.ToLower(matched[j].opt.Const)
	})

	out := make([]Option, len(matched))
	for i, m := range matched {
		out[i] = m.opt
	}
	return out
}

func matchScore(opt Option, q string) (int, bool) {
	title := strings.ToLower(opt.Title)
	code := strings.ToLower(opt.Const)
	switch {
	case title == q || code == q:
		return 0, true
	case strings.HasPrefix(title, q) || strings.HasPrefix(code, q):
		return 1, true
	case wordBoundary(title, q):
		return 2, true
	case strings.Contains(title, q) || strings.Contains(code, q):
		return 3, true
	default:
		return 0, false
	}
}

// wordBoundary reports whether q starts a word after the first. The first word
// is a prefix of the whole title, so the prefix tier already covers it.
// Separators are space, slash, and hyphen.
func wordBoundary(title, q string) bool {
	words := strings.FieldsFunc(title, func(r rune) bool {
		return r == ' ' || r == '/' || r == '-'
	})
	if len(words) < 2 {
		return false
	}
	for _, word := range words[1:] {
		if strings.HasPrefix(word, q) {
			return true
		}
	}
	return false
}

func pageOptions(options []Option, offset, limit int) []Option {
	if offset >= len(options) || limit <= 0 {
		return []Option{}
	}
	end := offset + limit
	if end > len(options) {
		end = len(options)
	}
	return options[offset:end]
}
