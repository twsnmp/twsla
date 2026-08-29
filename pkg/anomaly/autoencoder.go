package anomaly

import (
	"errors"
	"math"

	tensai "github.com/mattn/tensai"
	"github.com/mattn/tensai/layer"
	"github.com/mattn/tensai/loss"
	"github.com/mattn/tensai/model"
	"github.com/mattn/tensai/optim"
)

// AutoencoderDetector implements neural network Autoencoder anomaly detection using tensai
type AutoencoderDetector struct {
	epochs     int
	lr         float32
	net        *model.Sequential
	dim        int
	means      []float64
	stds       []float64
	normalized bool
}

// NewAutoencoderDetector creates an Autoencoder detector
func NewAutoencoderDetector() *AutoencoderDetector {
	return &AutoencoderDetector{
		epochs: 30,
		lr:     0.01,
	}
}

// Fit trains the Autoencoder to reconstruct normal feature vectors
func (d *AutoencoderDetector) Fit(vectors [][]float64) error {
	if len(vectors) == 0 {
		return errors.New("empty vectors")
	}
	rows := len(vectors)
	cols := len(vectors[0])
	if cols == 0 {
		return errors.New("zero feature dimension")
	}
	d.dim = cols

	cleaned := make([][]float64, rows)
	for i, v := range vectors {
		cv := make([]float64, cols)
		for j := 0; j < cols && j < len(v); j++ {
			if !math.IsNaN(v[j]) && !math.IsInf(v[j], 0) {
				cv[j] = v[j]
			}
		}
		cleaned[i] = cv
	}

	// Calculate mean and std for feature normalization
	d.means = make([]float64, cols)
	d.stds = make([]float64, cols)
	for _, v := range cleaned {
		for j, val := range v {
			d.means[j] += val
		}
	}
	for j := range d.means {
		d.means[j] /= float64(rows)
	}
	for _, v := range cleaned {
		for j, val := range v {
			diff := val - d.means[j]
			d.stds[j] += diff * diff
		}
	}
	for j := range d.stds {
		variance := d.stds[j] / float64(rows)
		if variance < 1e-8 {
			d.stds[j] = 1.0
		} else {
			d.stds[j] = math.Sqrt(variance)
		}
	}
	d.normalized = true

	// Build Matrix data
	data := make([]tensai.Float, rows*cols)
	for i, v := range cleaned {
		for j, val := range v {
			normVal := (val - d.means[j]) / d.stds[j]
			data[i*cols+j] = tensai.Float(normVal)
		}
	}
	mat, err := tensai.NewMatrixFromSlice(rows, cols, data)
	if err != nil {
		return err
	}

	// Bottleneck dimension
	bottleneck := cols / 4
	if bottleneck < 2 {
		bottleneck = 2
	}
	if bottleneck > 32 {
		bottleneck = 32
	}

	net := model.NewSequential()
	net.Add(layer.NewDense(bottleneck))
	net.Add(&layer.Tanh{})
	net.Add(layer.NewDense(cols))

	if err := net.Compile(cols, loss.MeanSquaredError{}, optim.NewAdam(d.lr)); err != nil {
		return err
	}

	epochs := d.epochs
	if rows > 1000 {
		epochs = 20
	}
	if err := net.Fit(mat, mat, epochs); err != nil {
		return err
	}

	d.net = net
	return nil
}

// Score calculates the reconstruction error (MSE)
func (d *AutoencoderDetector) Score(vector []float64) float64 {
	if d.net == nil || len(vector) != d.dim {
		return 0.0
	}

	normVec := make([]tensai.Float, d.dim)
	for j := 0; j < d.dim; j++ {
		val := 0.0
		if j < len(vector) && !math.IsNaN(vector[j]) && !math.IsInf(vector[j], 0) {
			val = vector[j]
		}
		if d.normalized {
			val = (val - d.means[j]) / d.stds[j]
		}
		normVec[j] = tensai.Float(val)
	}

	inMat, err := tensai.NewMatrixFromSlice(1, d.dim, normVec)
	if err != nil {
		return 0.0
	}

	outMat, err := d.net.Predict(inMat)
	if err != nil {
		return 0.0
	}

	var mse float64
	for j := 0; j < d.dim; j++ {
		diff := float64(normVec[j] - outMat.Data[j])
		mse += diff * diff
	}
	score := mse / float64(d.dim)
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return 0.0
	}
	return score
}
