package pgdb

import "strconv"

// Args collects bind arguments and hands out their numbered placeholders, so
// queries built from fragments bind in any order.
type Args []any

// Add appends value and returns its placeholder.
func (a *Args) Add(value any) string {
	*a = append(*a, value)
	return "$" + strconv.Itoa(len(*a))
}
