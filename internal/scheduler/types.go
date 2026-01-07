package scheduler

// GPURequest describes how many cards and what percent per card a pod requests.
type GPURequest struct {
	NumCards       int
	PercentPerCard int64
}

// GPUAllocationInfo stores which node and indices were selected during Reserve.
type GPUAllocationInfo struct {
	NodeName      string
	SelectedCards []int
	RequiredRatio int64
}

// devicesToIndices is a small helper converting device IDs to indices.
// For test purposes we provide a best-effort mapping; production logic lives
// in the build-tagged plugin implementation.
func devicesToIndices(devices []string) []int {
	out := make([]int, len(devices))
	for i := range devices {
		out[i] = i
	}
	return out
}
