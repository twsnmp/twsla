package anomaly

import (
	"errors"
	"math"
	"sort"
)

// LOFDetector computes Local Outlier Factor
type LOFDetector struct {
	k         int
	trainData [][]float64
	kDist     []float64
	lrd       []float64
}

// NewLOFDetector creates a new LOF detector with specified k
func NewLOFDetector(k int) *LOFDetector {
	if k < 2 {
		k = 5
	}
	return &LOFDetector{k: k}
}

type neighbor struct {
	idx  int
	dist float64
}

// Fit computes k-distances and local reachability densities for all training points
func (d *LOFDetector) Fit(vectors [][]float64) error {
	n := len(vectors)
	if n == 0 {
		return errors.New("empty vectors")
	}
	dim := len(vectors[0])
	cleaned := make([][]float64, n)
	for i, v := range vectors {
		cv := make([]float64, dim)
		for j := 0; j < dim && j < len(v); j++ {
			if !math.IsNaN(v[j]) && !math.IsInf(v[j], 0) {
				cv[j] = v[j]
			}
		}
		cleaned[i] = cv
	}

	k := d.k
	if k >= n {
		k = max(1, n-1)
		d.k = k
	}

	d.trainData = cleaned
	d.kDist = make([]float64, n)
	d.lrd = make([]float64, n)

	// Compute distance matrix / k-nearest neighbors for all points
	neighbors := make([][]neighbor, n)
	for i := 0; i < n; i++ {
		list := make([]neighbor, 0, n)
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			dist := safeEuclideanDist(cleaned[i], cleaned[j])
			list = append(list, neighbor{idx: j, dist: dist})
		}
		sort.Slice(list, func(a, b int) bool {
			return list[a].dist < list[b].dist
		})

		if len(list) >= k {
			d.kDist[i] = list[k-1].dist
			neighbors[i] = list[:k]
		} else if len(list) > 0 {
			d.kDist[i] = list[len(list)-1].dist
			neighbors[i] = list
		}
	}

	// Compute LRD for each point
	for i := 0; i < n; i++ {
		var sumReachDist float64
		nn := neighbors[i]
		for _, nb := range nn {
			reachDist := math.Max(d.kDist[nb.idx], nb.dist)
			sumReachDist += reachDist
		}
		if sumReachDist > 1e-12 {
			d.lrd[i] = float64(len(nn)) / sumReachDist
		} else {
			d.lrd[i] = 1e6
		}
	}

	return nil
}

// Score calculates the LOF score for a vector
func (d *LOFDetector) Score(vector []float64) float64 {
	n := len(d.trainData)
	if n == 0 || d.k == 0 {
		return 0.0
	}
	k := d.k

	list := make([]neighbor, 0, n)
	for j := 0; j < n; j++ {
		dist := safeEuclideanDist(vector, d.trainData[j])
		list = append(list, neighbor{idx: j, dist: dist})
	}
	sort.Slice(list, func(a, b int) bool {
		return list[a].dist < list[b].dist
	})

	if len(list) > k {
		list = list[:k]
	}

	var sumReachDist float64
	for _, nb := range list {
		reachDist := math.Max(d.kDist[nb.idx], nb.dist)
		sumReachDist += reachDist
	}

	var lrdP float64
	if sumReachDist > 1e-12 {
		lrdP = float64(len(list)) / sumReachDist
	} else {
		lrdP = 1e6
	}

	var sumLrdRatio float64
	for _, nb := range list {
		sumLrdRatio += d.lrd[nb.idx] / lrdP
	}

	score := sumLrdRatio / float64(len(list))
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return 1.0
	}
	return score
}
