package anomaly

import (
	"math"
	"math/rand"
	"testing"
)

func TestComputeCosineSimilarityMatrix(t *testing.T) {
	// Simple known vectors
	vecs := [][]float64{
		{1.0, 0.0, 0.0},
		{0.0, 1.0, 0.0},
		{1.0, 1.0, 0.0},
	}

	sims := ComputeCosineSimilarityMatrix(vecs, true)
	if len(sims) != 3 || len(sims[0]) != 3 {
		t.Fatalf("unexpected similarity matrix shape: %dx%d", len(sims), len(sims[0]))
	}

	// sims[0][0] should be 1.0
	if math.Abs(sims[0][0]-1.0) > 1e-4 {
		t.Errorf("sims[0][0] = %f, want 1.0", sims[0][0])
	}
	// sims[0][1] should be 0.0 (orthogonal)
	if math.Abs(sims[0][1]-0.0) > 1e-4 {
		t.Errorf("sims[0][1] = %f, want 0.0", sims[0][1])
	}
	// sims[0][2] should be 1/sqrt(2) ≈ 0.7071
	expected := 1.0 / math.Sqrt(2.0)
	if math.Abs(sims[0][2]-expected) > 1e-4 {
		t.Errorf("sims[0][2] = %f, want %f", sims[0][2], expected)
	}

	// Test larger matrix comparing CPU and GPU (or fallback)
	rng := rand.New(rand.NewSource(42))
	n := 128
	dim := 64
	largeVecs := make([][]float64, n)
	for i := 0; i < n; i++ {
		v := make([]float64, dim)
		for j := 0; j < dim; j++ {
			v[j] = rng.Float64()
		}
		largeVecs[i] = v
	}

	simsCPU := ComputeCosineSimilarityMatrix(largeVecs, true)
	simsAuto := ComputeCosineSimilarityMatrix(largeVecs, false)

	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			diff := math.Abs(simsCPU[i][j] - simsAuto[i][j])
			if diff > 1e-3 {
				t.Fatalf("mismatch at (%d, %d): cpu=%f, auto=%f", i, j, simsCPU[i][j], simsAuto[i][j])
			}
		}
	}
}

func TestComputePairwiseDistanceMatrix(t *testing.T) {
	vecs := [][]float64{
		{0.0, 0.0},
		{3.0, 4.0}, // dist to 0,0 is 5.0
		{0.0, 4.0}, // dist to 0,0 is 4.0
	}

	dists := ComputePairwiseDistanceMatrix(vecs, true)
	if len(dists) != 3 || len(dists[0]) != 3 {
		t.Fatalf("unexpected distance matrix shape: %dx%d", len(dists), len(dists[0]))
	}

	if math.Abs(dists[0][0]) > 1e-4 {
		t.Errorf("dists[0][0] = %f, want 0.0", dists[0][0])
	}
	if math.Abs(dists[0][1]-5.0) > 1e-4 {
		t.Errorf("dists[0][1] = %f, want 5.0", dists[0][1])
	}
	if math.Abs(dists[0][2]-4.0) > 1e-4 {
		t.Errorf("dists[0][2] = %f, want 4.0", dists[0][2])
	}
}
