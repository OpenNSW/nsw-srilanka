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

// rank scores a lowercased query as: exact title, title prefix, exact or
// prefix const, title contains, const contains. Ties keep artifact order.
func rank(options []Option, query string) []Option {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return options
	}

	type scored struct {
		opt   Option
		score int
		idx   int
	}
	matched := make([]scored, 0, len(options))
	for i, opt := range options {
		score, ok := matchScore(opt, q)
		if !ok {
			continue
		}
		matched = append(matched, scored{opt: opt, score: score, idx: i})
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].score != matched[j].score {
			return matched[i].score < matched[j].score
		}
		return matched[i].idx < matched[j].idx
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
	case title == q:
		return 0, true
	case strings.HasPrefix(title, q):
		return 1, true
	case code == q || strings.HasPrefix(code, q):
		return 2, true
	case strings.Contains(title, q):
		return 3, true
	case strings.Contains(code, q):
		return 4, true
	default:
		return 0, false
	}
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
