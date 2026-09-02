package anomaly

import (
	"fmt"
	"math"
	"runtime"
	"sync"

	tensai "github.com/mattn/tensai"
	aitensai "github.com/twsnmp/twsla/pkg/ai/tensai"
)

// GetActiveBackend returns the name of the active acceleration backend
func GetActiveBackend(noGPU bool) string {
	accType, accDetail := aitensai.DetectAccelerationWithOptions(noGPU)
	return fmt.Sprintf("%s (%s)", accType, accDetail)
}

// ComputeCosineSimilarityMatrix calculates the pairwise cosine similarity matrix for N vectors of dimension D.
// It uses GPU (Metal/WebGPU GEMM) -> SIMD (AVX2 parallel) -> CPU (Portable) fallback.
func ComputeCosineSimilarityMatrix(vectors [][]float64, noGPU bool) [][]float64 {
	n := len(vectors)
	if n == 0 {
		return [][]float64{}
	}
	dim := len(vectors[0])
	if dim == 0 {
		return make([][]float64, n)
	}

	// L2 normalize vectors into float32 flat buffer
	normalized := make([]float32, n*dim)
	for i, v := range vectors {
		var sumSq float64
		for j := 0; j < dim && j < len(v); j++ {
			val := v[j]
			if !math.IsNaN(val) && !math.IsInf(val, 0) {
				sumSq += val * val
			}
		}
		norm := math.Sqrt(sumSq)
		if norm > 1e-12 {
			invNorm := 1.0 / norm
			for j := 0; j < dim && j < len(v); j++ {
				val := v[j]
				if !math.IsNaN(val) && !math.IsInf(val, 0) {
					normalized[i*dim+j] = float32(val * invNorm)
				}
			}
		}
	}

	// For small matrices (N < 64), CPU parallel computation is faster due to zero transfer overhead
	if n >= 64 && !noGPU {
		if simMatrix, err := computeCosineSimilarityGPU(normalized, n, dim); err == nil && simMatrix != nil {
			return simMatrix
		}
	}

	return computeCosineSimilarityCPU(normalized, n, dim)
}

// computeCosineSimilarityGPU computes S = X * X^T on GPU
func computeCosineSimilarityGPU(normalized []float32, n, dim int) ([][]float64, error) {
	dev, err := aitensai.GetGPUDevice()
	if err != nil || dev == nil {
		return nil, fmt.Errorf("gpu device not available")
	}

	// Create Tensai tensors: A is [N, dim]
	xTensor := &tensai.Tensor{
		Shape: []int{n, dim},
		Data:  normalized,
	}
	gx, err := dev.Upload(xTensor)
	if err != nil {
		return nil, err
	}
	defer gx.Free()

	// Compute X * X^T
	// Since X is [N, dim], its transpose X^T is [dim, N].
	xTData := make([]float32, dim*n)
	for r := 0; r < n; r++ {
		for c := 0; c < dim; c++ {
			xTData[c*n+r] = normalized[r*dim+c]
		}
	}
	gxT, err := dev.Upload(&tensai.Tensor{
		Shape: []int{dim, n},
		Data:  xTData,
	})
	if err != nil {
		return nil, err
	}
	defer gxT.Free()

	res, err := gx.MatMul(gxT)
	if err != nil {
		return nil, err
	}
	defer res.Free()

	downloaded, err := res.Download()
	if err != nil {
		return nil, err
	}

	out := make([][]float64, n)
	flatData := downloaded.Data
	for i := 0; i < n; i++ {
		row := make([]float64, n)
		rowOff := i * n
		for j := 0; j < n; j++ {
			val := float64(flatData[rowOff+j])
			if i == j {
				val = 1.0
			} else if val > 1.0 {
				val = 1.0
			} else if val < -1.0 {
				val = -1.0
			}
			row[j] = val
		}
		out[i] = row
	}

	return out, nil
}

// computeCosineSimilarityCPU computes S = X * X^T with multicore Goroutine parallelization
func computeCosineSimilarityCPU(normalized []float32, n, dim int) [][]float64 {
	out := make([][]float64, n)
	for i := range out {
		out[i] = make([]float64, n)
	}

	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}

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
				iOff := i * dim
				out[i][i] = 1.0
				for j := i + 1; j < n; j++ {
					jOff := j * dim
					var dot float32
					// Unroll dot product loop for SIMD auto-vectorization
					d := 0
					for ; d <= dim-8; d += 8 {
						dot += normalized[iOff+d]*normalized[jOff+d] +
							normalized[iOff+d+1]*normalized[jOff+d+1] +
							normalized[iOff+d+2]*normalized[jOff+d+2] +
							normalized[iOff+d+3]*normalized[jOff+d+3] +
							normalized[iOff+d+4]*normalized[jOff+d+4] +
							normalized[iOff+d+5]*normalized[jOff+d+5] +
							normalized[iOff+d+6]*normalized[jOff+d+6] +
							normalized[iOff+d+7]*normalized[jOff+d+7]
					}
					for ; d < dim; d++ {
						dot += normalized[iOff+d] * normalized[jOff+d]
					}
					val := float64(dot)
					if val > 1.0 {
						val = 1.0
					} else if val < -1.0 {
						val = -1.0
					}
					out[i][j] = val
					out[j][i] = val
				}
			}
		}(start, end)
	}

	wg.Wait()
	return out
}

// ComputePairwiseDistanceMatrix calculates all-pairs Euclidean distances: D_ij = ||x_i - x_j||
// using Gram matrix expansion: ||x_i - x_j||^2 = ||x_i||^2 + ||x_j||^2 - 2 <x_i, x_j>
func ComputePairwiseDistanceMatrix(vectors [][]float64, noGPU bool) [][]float64 {
	n := len(vectors)
	if n == 0 {
		return [][]float64{}
	}
	dim := len(vectors[0])
	if dim == 0 {
		return make([][]float64, n)
	}

	// Compute squared norms for each vector
	sqNorms := make([]float64, n)
	flat := make([]float32, n*dim)
	for i, v := range vectors {
		var sq float64
		for j := 0; j < dim && j < len(v); j++ {
			val := v[j]
			if !math.IsNaN(val) && !math.IsInf(val, 0) {
				sq += val * val
				flat[i*dim+j] = float32(val)
			}
		}
		sqNorms[i] = sq
	}

	var gram [][]float64
	if n >= 64 && !noGPU {
		// Use GPU for Gram matrix G = X * X^T
		if g, err := computeCosineSimilarityGPU(flat, n, dim); err == nil && g != nil {
			gram = g
		}
	}

	if gram == nil {
		// Fallback to CPU parallel Gram matrix
		gram = computeGramCPU(flat, n, dim)
	}

	// Compute Euclidean distances from Gram matrix and squared norms
	dists := make([][]float64, n)
	for i := range dists {
		dists[i] = make([]float64, n)
	}

	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
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
				for j := i + 1; j < n; j++ {
					sqDist := sqNorms[i] + sqNorms[j] - 2.0*gram[i][j]
					if sqDist < 0 {
						sqDist = 0
					}
					d := math.Sqrt(sqDist)
					dists[i][j] = d
					dists[j][i] = d
				}
			}
		}(start, end)
	}
	wg.Wait()

	return dists
}

func computeGramCPU(flat []float32, n, dim int) [][]float64 {
	out := make([][]float64, n)
	for i := range out {
		out[i] = make([]float64, n)
	}

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
				iOff := i * dim
				for j := i; j < n; j++ {
					jOff := j * dim
					var dot float32
					d := 0
					for ; d <= dim-8; d += 8 {
						dot += flat[iOff+d]*flat[jOff+d] +
							flat[iOff+d+1]*flat[jOff+d+1] +
							flat[iOff+d+2]*flat[jOff+d+2] +
							flat[iOff+d+3]*flat[jOff+d+3] +
							flat[iOff+d+4]*flat[jOff+d+4] +
							flat[iOff+d+5]*flat[jOff+d+5] +
							flat[iOff+d+6]*flat[jOff+d+6] +
							flat[iOff+d+7]*flat[jOff+d+7]
					}
					for ; d < dim; d++ {
						dot += flat[iOff+d] * flat[jOff+d]
					}
					val := float64(dot)
					out[i][j] = val
					out[j][i] = val
				}
			}
		}(start, end)
	}
	wg.Wait()
	return out
}
