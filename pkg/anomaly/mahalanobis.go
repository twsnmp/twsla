package anomaly

import (
	"errors"
	"math"
)

// MahalanobisDetector detects anomalies using Mahalanobis distance
type MahalanobisDetector struct {
	mean      []float64
	invCov    [][]float64
	invStdDev []float64 // for high-dimensional diagonal covariance
	dim       int
	useDiag   bool
}

// NewMahalanobisDetector creates a Mahalanobis distance anomaly detector
func NewMahalanobisDetector() *MahalanobisDetector {
	return &MahalanobisDetector{}
}

// Fit computes the mean vector and inverted covariance matrix
func (d *MahalanobisDetector) Fit(vectors [][]float64) error {
	n := len(vectors)
	if n == 0 {
		return errors.New("empty vectors")
	}
	dim := len(vectors[0])
	if dim == 0 {
		return errors.New("zero feature dimension")
	}
	d.dim = dim

	// Clean inputs
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

	// Mean vector
	d.mean = make([]float64, dim)
	for _, v := range cleaned {
		for j := 0; j < dim; j++ {
			d.mean[j] += v[j]
		}
	}
	for j := 0; j < dim; j++ {
		d.mean[j] /= float64(n)
	}

	// For high dimensions (> 64), use diagonal variance for speed and numerical stability
	if dim > 64 || dim >= n {
		d.useDiag = true
		d.invStdDev = make([]float64, dim)
		denom := float64(max(1, n-1))
		for _, v := range cleaned {
			for j := 0; j < dim; j++ {
				diff := v[j] - d.mean[j]
				d.invStdDev[j] += diff * diff
			}
		}
		for j := 0; j < dim; j++ {
			variance := (d.invStdDev[j] / denom) + 1e-4
			d.invStdDev[j] = 1.0 / math.Sqrt(variance)
		}
		return nil
	}

	// Full covariance matrix for lower dimensional vectors
	d.useDiag = false
	cov := make([][]float64, dim)
	for i := range cov {
		cov[i] = make([]float64, dim)
	}

	for _, v := range cleaned {
		for i := 0; i < dim; i++ {
			diffI := v[i] - d.mean[i]
			for j := 0; j < dim; j++ {
				diffJ := v[j] - d.mean[j]
				cov[i][j] += diffI * diffJ
			}
		}
	}

	denom := float64(max(1, n-1))
	for i := 0; i < dim; i++ {
		for j := 0; j < dim; j++ {
			cov[i][j] /= denom
		}
		cov[i][i] += 1e-4
	}

	inv, err := invertMatrix(cov, dim)
	if err != nil {
		// Fallback to diagonal
		d.useDiag = true
		d.invStdDev = make([]float64, dim)
		for j := 0; j < dim; j++ {
			d.invStdDev[j] = 1.0 / math.Sqrt(cov[j][j])
		}
		return nil
	}
	d.invCov = inv
	return nil
}

// Score computes the Mahalanobis distance from the mean vector
func (d *MahalanobisDetector) Score(vector []float64) float64 {
	if len(d.mean) == 0 {
		return 0.0
	}
	dim := d.dim
	diff := make([]float64, dim)
	for i := 0; i < dim; i++ {
		val := 0.0
		if i < len(vector) && !math.IsNaN(vector[i]) && !math.IsInf(vector[i], 0) {
			val = vector[i]
		}
		diff[i] = val - d.mean[i]
	}

	if d.useDiag {
		var sumSq float64
		for i := 0; i < dim; i++ {
			z := diff[i] * d.invStdDev[i]
			sumSq += z * z
		}
		return math.Sqrt(sumSq)
	}

	var distSq float64
	for i := 0; i < dim; i++ {
		var rowSum float64
		for j := 0; j < dim; j++ {
			rowSum += diff[j] * d.invCov[j][i]
		}
		distSq += diff[i] * rowSum
	}
	if distSq < 0 || math.IsNaN(distSq) {
		return 0.0
	}
	return math.Sqrt(distSq)
}

func invertMatrix(a [][]float64, n int) ([][]float64, error) {
	aug := make([][]float64, n)
	for i := 0; i < n; i++ {
		aug[i] = make([]float64, 2*n)
		for j := 0; j < n; j++ {
			aug[i][j] = a[i][j]
		}
		aug[i][n+i] = 1.0
	}

	for i := 0; i < n; i++ {
		maxRow := i
		maxVal := math.Abs(aug[i][i])
		for k := i + 1; k < n; k++ {
			if math.Abs(aug[k][i]) > maxVal {
				maxVal = math.Abs(aug[k][i])
				maxRow = k
			}
		}
		if maxVal < 1e-12 {
			return nil, errors.New("singular matrix")
		}
		aug[i], aug[maxRow] = aug[maxRow], aug[i]

		pivot := aug[i][i]
		for j := 0; j < 2*n; j++ {
			aug[i][j] /= pivot
		}

		for k := 0; k < n; k++ {
			if k != i {
				factor := aug[k][i]
				for j := 0; j < 2*n; j++ {
					aug[k][j] -= factor * aug[i][j]
				}
			}
		}
	}

	inv := make([][]float64, n)
	for i := 0; i < n; i++ {
		inv[i] = make([]float64, n)
		for j := 0; j < n; j++ {
			inv[i][j] = aug[i][n+j]
		}
	}
	return inv, nil
}
