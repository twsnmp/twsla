package anomaly

import (
	"errors"
	"math"
	"runtime"
	"sort"
	"sync"
)

// KNNDetector computes anomaly score based on k-nearest neighbor distance
type KNNDetector struct {
	k         int
	noGPU     bool
	trainData [][]float64
}

// NewKNNDetector creates a new k-NN distance anomaly detector
func NewKNNDetector(k int) *KNNDetector {
	return NewKNNDetectorWithOptions(k, false)
}

// NewKNNDetectorWithOptions creates a new k-NN distance anomaly detector with GPU option
func NewKNNDetectorWithOptions(k int, noGPU bool) *KNNDetector {
	if k < 1 {
		k = 5
	}
	return &KNNDetector{k: k, noGPU: noGPU}
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

// Score computes the mean distance to the k-nearest neighbors for a single vector
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

// ScoreBatch computes anomaly scores for multiple vectors using GPU/SIMD accelerated distance matrix
func (d *KNNDetector) ScoreBatch(vectors [][]float64) []float64 {
	n := len(vectors)
	if n == 0 {
		return []float64{}
	}
	trainN := len(d.trainData)
	if trainN == 0 {
		return make([]float64, n)
	}

	k := d.k
	if k > trainN {
		k = trainN
	}

	// If evaluating trainData against itself, use all-pairs distance matrix directly
	if n == trainN {
		distMatrix := ComputePairwiseDistanceMatrix(d.trainData, d.noGPU)
		scores := make([]float64, n)

		workers := runtime.GOMAXPROCS(0)
		var wg sync.WaitGroup
		chunkSize := (n + workers - 1) / workers

		for w := 0; w < workers; w++ {
			start := w * chunkSize
			end := start + chunkSize
			if end > n {
				end = n
			}
			if start >= end {
				continue
			}

			wg.Add(1)
			go func(rStart, rEnd int) {
				defer wg.Done()
				for i := rStart; i < rEnd; i++ {
					row := make([]float64, n)
					copy(row, distMatrix[i])
					sort.Float64s(row)

					// First distance is to itself (0.0), so use the next k distances
					var sumDist float64
					count := 0
					for idx := 1; idx <= k && idx < n; idx++ {
						sumDist += row[idx]
						count++
					}
					if count > 0 {
						scores[i] = sumDist / float64(count)
					}
				}
			}(start, end)
		}
		wg.Wait()
		return scores
	}

	// For different query vectors, fall back to parallel individual scoring
	scores := make([]float64, n)
	for i, v := range vectors {
		scores[i] = d.Score(v)
	}
	return scores
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
