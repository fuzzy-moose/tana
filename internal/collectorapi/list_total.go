package collectorapi

// validListTotal reports whether a list response's total is consistent with the requested page.
func validListTotal(total, offset int64, count int) bool {
	return total >= 0 && (count == 0 || total >= offset+int64(count))
}
