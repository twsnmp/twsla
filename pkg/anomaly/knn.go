package anomaly

import (
	"errors"
	"math"
	"sort"
)

// KNNDetector computes anomaly score based on k-nearest neighbor distance
type KNNDetector struct {
	k         int
	trainData [][]float64
}

// NewKNNDetector creates a new k-NN distance anomaly detector
func NewKNNDetector(k int) *KNNDetector {
	if k < 1 {
		k = 5
	}
	return &KNNDetector{k: k}
}

// Fit stores training feature vectors
func (d *KNNDetector) Fit(vectors [][]float64) error {
	if len(vectors) == 0 {
		return errors.New("empty vectors")
	}
	n := len(vectors)
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

	d.trainData = cleaned
	if d.k >= len(cleaned) {
		d.k = max(1, len(cleaned)-1)
	}
	return nil
}

// Score computes the mean distance to the k-nearest neighbors
func (d *KNNDetector) Score(vector []float64) float64 {
	n := len(d.trainData)
	if n == 0 {
		return 0.0
	}
	k := d.k
	if k > n {
		k = n
	}

	dists := make([]float64, n)
	for i, v := range d.trainData {
		dists[i] = safeEuclideanDist(vector, v)
	}
	sort.Float64s(dists)

	var sumDist float64
	for i := 0; i < k; i++ {
		sumDist += dists[i]
	}
	return sumDist / float64(k)
}

func safeEuclideanDist(a, b []float64) float64 {
	var sum float64
	dim := min(len(a), len(b))
	for i := 0; i < dim; i++ {
		va, vb := 0.0, 0.0
		if !math.IsNaN(a[i]) && !math.IsInf(a[i], 0) {
			va = a[i]
		}
		if !math.IsNaN(b[i]) && !math.IsInf(b[i], 0) {
			vb = b[i]
		}
		d := va - vb
		sum += d * d
	}
	return math.Sqrt(sum)
}
