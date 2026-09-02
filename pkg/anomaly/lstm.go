package anomaly

import (
	"errors"
	"math"

	tensai "github.com/mattn/tensai"
	"github.com/mattn/tensai/layer"
	"github.com/mattn/tensai/loss"
	"github.com/mattn/tensai/model"
	"github.com/mattn/tensai/optim"
	aitensai "github.com/twsnmp/twsla/pkg/ai/tensai"
)

// LSTMDetector detects sequential transition anomalies
type LSTMDetector struct {
	noGPU      bool
	net        *model.Sequential
	dim        int
	prevVector []float64
}

// NewLSTMDetector creates an LSTM/Sequence transition anomaly detector
func NewLSTMDetector() *LSTMDetector {
	return NewLSTMDetectorWithOptions(false)
}

// NewLSTMDetectorWithOptions creates an LSTM detector with GPU option
func NewLSTMDetectorWithOptions(noGPU bool) *LSTMDetector {
	return &LSTMDetector{noGPU: noGPU}
}

// Fit trains the transition predictor model (predict X_{t+1} from X_t)
func (d *LSTMDetector) Fit(vectors [][]float64) error {
	if len(vectors) < 2 {
		return errors.New("need at least 2 vectors for sequence learning")
	}
	dim := len(vectors[0])
	if dim == 0 {
		return errors.New("zero feature dimension")
	}
	d.dim = dim

	if !d.noGPU {
		if dev, err := aitensai.GetGPUDevice(); err == nil && dev != nil {
			tensai.UseAccelerator(dev)
		}
	}

	// Limit sample size for sequence training to ensure interactive responsiveness
	maxSamples := 300
	step := 1
	if len(vectors) > maxSamples {
		step = len(vectors) / maxSamples
		if step < 1 {
			step = 1
		}
	}

	var sampled [][]float64
	for i := 0; i < len(vectors); i += step {
		cv := make([]float64, dim)
		for j := 0; j < dim && j < len(vectors[i]); j++ {
			if !math.IsNaN(vectors[i][j]) && !math.IsInf(vectors[i][j], 0) {
				cv[j] = vectors[i][j]
			}
		}
		sampled = append(sampled, cv)
	}

	samples := len(sampled) - 1
	if samples < 1 {
		return errors.New("not enough sampled vectors")
	}

	inData := make([]tensai.Float, samples*dim)
	tgtData := make([]tensai.Float, samples*dim)
	for i := 0; i < samples; i++ {
		for j := 0; j < dim; j++ {
			inData[i*dim+j] = tensai.Float(sampled[i][j])
			tgtData[i*dim+j] = tensai.Float(sampled[i+1][j])
		}
	}

	inMat, err := tensai.NewMatrixFromSlice(samples, dim, inData)
	if err != nil {
		return err
	}
	tgtMat, err := tensai.NewMatrixFromSlice(samples, dim, tgtData)
	if err != nil {
		return err
	}

	hidden := dim / 4
	if hidden < 4 {
		hidden = 4
	}
	if hidden > 32 {
		hidden = 32
	}

	net := model.NewSequential()
	net.Add(layer.NewDense(hidden))
	net.Add(&layer.Tanh{})
	net.Add(layer.NewDense(dim))

	if err := net.Compile(dim, loss.MeanSquaredError{}, optim.NewAdam(0.01)); err != nil {
		return err
	}

	epochs := 20
	for e := 1; e <= epochs; e++ {
		if _, err := net.FitStep(inMat, tgtMat); err != nil {
			return err
		}
	}

	d.net = net
	return nil
}

// Score computes the sequential transition prediction error
func (d *LSTMDetector) Score(vector []float64) float64 {
	if d.net == nil || len(vector) != d.dim {
		return 0.0
	}
	cleanVec := make([]float64, d.dim)
	for j := 0; j < d.dim && j < len(vector); j++ {
		if !math.IsNaN(vector[j]) && !math.IsInf(vector[j], 0) {
			cleanVec[j] = vector[j]
		}
	}

	if d.prevVector == nil {
		d.prevVector = make([]float64, d.dim)
		copy(d.prevVector, cleanVec)
		return 0.0
	}

	inData := make([]tensai.Float, d.dim)
	for j, v := range d.prevVector {
		inData[j] = tensai.Float(v)
	}

	inMat, err := tensai.NewMatrixFromSlice(1, d.dim, inData)
	if err != nil {
		return 0.0
	}

	predMat, err := d.net.Predict(inMat)
	if err != nil {
		return 0.0
	}

	var mse float64
	for j := 0; j < d.dim; j++ {
		diff := cleanVec[j] - float64(predMat.Data[j])
		mse += diff * diff
	}
	copy(d.prevVector, cleanVec)

	score := mse / float64(d.dim)
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return 0.0
	}
	return score
}
