package util

func Abs(x int) int { // Helper func to return absolute difference between tracks
	if x < 0 {
		return -x
	}
	return x
}