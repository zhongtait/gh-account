package utils

import "strings"

// SuggestCandidate finds the closest string in candidates within an edit distance of 2.
// Returns empty string if no good match is found.
func SuggestCandidate(input string, candidates []string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" || len(candidates) == 0 {
		return ""
	}

	bestDistance := 3
	var bestMatch string

	for _, cand := range candidates {
		candLower := strings.ToLower(strings.TrimSpace(cand))
		if candLower == input {
			continue
		}
		dist := levenshteinDistance(input, candLower)
		if dist < bestDistance {
			bestDistance = dist
			bestMatch = cand
		}
	}

	return bestMatch
}

func levenshteinDistance(s, t string) int {
	d := make([][]int, len(s)+1)
	for i := range d {
		d[i] = make([]int, len(t)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(s); i++ {
		for j := 1; j <= len(t); j++ {
			cost := 0
			if s[i-1] != t[j-1] {
				cost = 1
			}
			d[i][j] = min(
				d[i-1][j]+1,
				min(
					d[i][j-1]+1,
					d[i-1][j-1]+cost,
				),
			)
		}
	}
	return d[len(s)][len(t)]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
