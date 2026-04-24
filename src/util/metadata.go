package util

// Return absolute difference between tracks
func Abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}