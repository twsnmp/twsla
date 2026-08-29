package anomaly

import (
	"errors"
	"math"
)

// StatDetector detects anomalies using statistical Z-score
type StatDetector struct {
	means []float64
	stds  []float64
	dim   int
}

// NewStatDetector creates a Z-score statistical anomaly detector
func NewStatDetector() *StatDetector {
	return &StatDetector{}
}

// Fit computes the mean and standard deviation for each feature dimension
func (d *StatDetector) Fit(vectors [][]float64) error {
	n := len(vectors)
	if n == 0 {
		return errors.New("empty vectors")
	}
	dim := len(vectors[0])
	if dim == 0 {
		return errors.New("zero feature dimension")
	}
	d.dim = dim

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

	d.means = make([]float64, dim)
	d.stds = make([]float64, dim)

	for _, v := range cleaned {
		for j := 0; j < dim; j++ {
			d.means[j] += v[j]
		}
	}
	for j := 0; j < dim; j++ {
		d.means[j] /= float64(n)
	}

	for _, v := range cleaned {
		for j := 0; j < dim; j++ {
			diff := v[j] - d.means[j]
			d.stds[j] += diff * diff
		}
	}
	for j := 0; j < dim; j++ {
		variance := d.stds[j] / float64(max(1, n-1))
		if variance < 1e-8 {
			d.stds[j] = 1.0
		} else {
			d.stds[j] = math.Sqrt(variance)
		}
	}

	return nil
}

// Score computes the Euclidean/RMS Z-score across feature dimensions
func (d *StatDetector) Score(vector []float64) float64 {
	if len(d.means) == 0 {
		return 0.0
	}
	dim := d.dim
	var sumSq float64
	for j := 0; j < dim; j++ {
		val := 0.0
		if j < len(vector) && !math.IsNaN(vector[j]) && !math.IsInf(vector[j], 0) {
			val = vector[j]
		}
		z := (val - d.means[j]) / d.stds[j]
		sumSq += z * z
	}
	score := math.Sqrt(sumSq / float64(dim))
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return 0.0
	}
	return score
}
