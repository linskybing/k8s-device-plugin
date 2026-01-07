package scheduler

import (
	"testing"
)

func TestScore_TopNAverage(t *testing.T) {
	old := GetDeviceRemaining
	defer func() { GetDeviceRemaining = old }()

	// simulate a node with 4 devices with remaining percent values
	GetDeviceRemaining = func(nodeName string) (map[string]int, error) {
		return map[string]int{
			"gpu0": 100,
			"gpu1": 80,
			"gpu2": 60,
			"gpu3": 40,
		}, nil
	}

	// call the standalone scoring helper
	score, err := ScoreNodeTopNAverage("nodeA", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// top-2 are 100 and 80 -> avg = 90
	if score != 90 {
		t.Fatalf("expected score 90, got %d", score)
	}
}
