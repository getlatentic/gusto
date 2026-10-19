package cmdline

// closest is the name nearest to what was typed, when one is near enough to be a slip of the fingers.
func closest(typed string, candidates []string) (string, bool) {
	best, bestDistance := "", len(typed)
	for _, candidate := range candidates {
		if d := distance(typed, candidate); d < bestDistance {
			best, bestDistance = candidate, d
		}
	}
	limit := 2
	if len(typed) < 4 {
		limit = 1
	}
	return best, best != "" && bestDistance <= limit
}

// distance is the Levenshtein distance between two words.
func distance(a, b string) int {
	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current := make([]int, len(b)+1)
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous = current
	}
	return previous[len(b)]
}
