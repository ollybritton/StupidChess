package position

// abs returns the absolute value of a given integer.
func abs(x int) int {
	if x < 0 {
		return -x
	}

	return x
}
