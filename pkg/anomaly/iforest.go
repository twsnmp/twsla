package anomaly

import (
	"errors"

	go_iforest "github.com/codegaudi/go-iforest"
)

// IForestDetector wraps go-iforest for Isolation Forest anomaly detection
type IForestDetector struct {
	trees  int
	sample int
	forest *go_iforest.IForest
}

// NewIForestDetector creates a new Isolation Forest detector
func NewIForestDetector() *IForestDetector {
	return &IForestDetector{
		trees:  1000,
		sample: 256,
	}
}

// Fit trains the Isolation Forest on the vectors
func (d *IForestDetector) Fit(vectors [][]float64) error {
	if len(vectors) == 0 {
		return errors.New("empty vectors")
	}
	sample := d.sample
	if sample > len(vectors) {
		sample = len(vectors)
	}
	f, err := go_iforest.NewIForest(vectors, d.trees, sample)
	if err != nil {
		return err
	}
	d.forest = f
	return nil
}

// Score returns the anomaly score for a given vector
func (d *IForestDetector) Score(vector []float64) float64 {
	if d.forest == nil {
		return 0.0
	}
	return d.forest.CalculateAnomalyScore(vector)
}
