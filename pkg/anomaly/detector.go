package anomaly

import "fmt"

// Detector is the common interface for anomaly detection algorithms
type Detector interface {
	// Fit trains the detector on the provided feature vectors
	Fit(vectors [][]float64) error
	// Score returns the anomaly score for a given vector (higher score = more anomalous)
	Score(vector []float64) float64
}

// BatchScorer is an optional interface for detectors that support batch-accelerated scoring
type BatchScorer interface {
	ScoreBatch(vectors [][]float64) []float64
}

// Result represents an scored log entry
type Result struct {
	Index int
	Score float64
}

// ProgressCallback provides progress feedback during training/scoring
type ProgressCallback func(phase string, current, total int)

// NewDetector creates an anomaly detector based on algorithm name
func NewDetector(algo string) (Detector, error) {
	return NewDetectorWithOptions(algo, false)
}

// NewDetectorWithOptions creates an anomaly detector with options
func NewDetectorWithOptions(algo string, noGPU bool) (Detector, error) {
	switch algo {
	case "iforest", "isolation_forest", "":
		return NewIForestDetector(), nil
	case "autoencoder", "ae", "nn":
		return NewAutoencoderDetectorWithOptions(noGPU), nil
	case "lstm", "rnn":
		return NewLSTMDetectorWithOptions(noGPU), nil
	case "lof", "local_outlier_factor":
		return NewLOFDetectorWithOptions(10, noGPU), nil
	case "knn", "nearest_neighbors":
		return NewKNNDetectorWithOptions(5, noGPU), nil
	case "mahalanobis", "md":
		return NewMahalanobisDetector(), nil
	case "zscore", "stat", "iqr":
		return NewStatDetector(), nil
	default:
		return nil, fmt.Errorf("unknown anomaly detection algorithm: %q (supported: iforest, autoencoder, lstm, lof, knn, mahalanobis, zscore)", algo)
	}
}
