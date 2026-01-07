package scheduler

import (
	"fmt"
	"sort"
)

// ScoreNodeTopNAverage returns the average remaining percent of the top-N devices
// on the node. Returns an error if the node reports fewer than numCards devices
// or if fetching status fails.
func ScoreNodeTopNAverage(nodeName string, numCards int) (int, error) {
	m, err := GetDeviceRemaining(nodeName)
	if err != nil {
		return 0, err
	}
	if len(m) < numCards {
		return 0, fmt.Errorf("insufficient devices: need %d got %d", numCards, len(m))
	}
	var rems []int
	for _, r := range m {
		rems = append(rems, r)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(rems)))
	sum := 0
	for i := 0; i < numCards; i++ {
		sum += rems[i]
	}
	avg := sum / numCards
	if avg > 100 {
		avg = 100
	} else if avg < 0 {
		avg = 0
	}
	return avg, nil
}
