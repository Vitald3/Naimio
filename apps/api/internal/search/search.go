package search

import (
	"errors"
	"strings"
)

var ErrInvalid = errors.New("invalid search input")

// Input is transport- and provider-neutral. Domain repositories decide how to
// execute it; callers cannot provide SQL columns or order expressions.
type Input struct {
	Query string
	Sort  string
}

func Normalize(query, sort, defaultSort string, allowedSorts ...string) (Input, error) {
	query = strings.TrimSpace(query)
	if len([]rune(query)) > 120 {
		return Input{}, ErrInvalid
	}
	sort = strings.ToUpper(strings.TrimSpace(sort))
	if sort == "" {
		sort = defaultSort
	}
	for _, allowed := range allowedSorts {
		if sort == allowed {
			return Input{Query: query, Sort: sort}, nil
		}
	}
	return Input{}, ErrInvalid
}
