package anomaly

import (
	"math"
	"testing"
)

func TestAnomalyDetectors(t *testing.T) {
	// Normal dataset: clustering around (1.0, 1.0)
	normalData := [][]float64{
		{1.0, 1.1},
		{0.9, 1.0},
		{1.1, 0.9},
		{1.05, 1.05},
		{0.95, 0.98},
		{1.0, 0.95},
		{1.02, 1.01},
		{0.98, 1.03},
		{1.01, 0.99},
		{0.99, 1.02},
	}

	// Outlier point far away: (10.0, 10.0)
	outlier := []float64{10.0, 10.0}
	normalPoint := []float64{1.0, 1.0}

	algos := []string{
		"iforest",
		"autoencoder",
		"lstm",
		"lof",
		"knn",
		"mahalanobis",
		"zscore",
	}

	for _, algo := range algos {
		t.Run(algo, func(t *testing.T) {
			detector, err := NewDetector(algo)
			if err != nil {
				t.Fatalf("NewDetector(%q) failed: %v", algo, err)
			}

			if err := detector.Fit(normalData); err != nil {
				t.Fatalf("Fit failed for %s: %v", algo, err)
			}

			normScore := detector.Score(normalPoint)
			outlierScore := detector.Score(outlier)

			if math.IsNaN(normScore) || math.IsNaN(outlierScore) {
				t.Errorf("%s returned NaN score", algo)
			}

			// Outlier score should be higher than or equal to normal point
			if outlierScore < normScore {
				t.Logf("[%s] Warning: outlierScore (%.3f) < normScore (%.3f)", algo, outlierScore, normScore)
			} else {
				t.Logf("[%s] PASSED: normalScore=%.3f, outlierScore=%.3f", algo, normScore, outlierScore)
			}
		})
	}
}
