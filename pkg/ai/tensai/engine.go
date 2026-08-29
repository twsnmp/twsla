package tensai

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/mattn/tensai/tokenizer"
)

// Engine runs autoregressive text generation using tensai's pure Go kernels
type Engine struct {
	model *qwen
	tok   *tokenizer.Tokenizer
	imEnd int
	eot   int
	rng   *rand.Rand
	mu    sync.Mutex
}

// LoadEngine loads the full model from GGUF
func LoadEngine(modelPath string) (*Engine, error) {
	model, tok, err := loadQwenGGUF(modelPath, 8)
	if err != nil {
		return nil, err
	}

	imEnd := model.cfg.EOS
	if id, ok := tok.ID("<|im_end|>"); ok {
		imEnd = id
	}
	eot := model.cfg.EOS
	if id, ok := tok.ID("<|endoftext|>"); ok {
		eot = id
	}

	return &Engine{
		model: model,
		tok:   tok,
		imEnd: imEnd,
		eot:   eot,
		rng:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

func sampleToken(logits []float32, temp float64, topP float64, rng *rand.Rand) int {
	if temp <= 0 {
		bestID := 0
		bestVal := logits[0]
		for i := 1; i < len(logits); i++ {
			if logits[i] > bestVal {
				bestVal = logits[i]
				bestID = i
			}
		}
		return bestID
	}

	var maxLogit float32 = -1e9
	for _, l := range logits {
		if l > maxLogit {
			maxLogit = l
		}
	}

	probs := make([]float64, len(logits))
	var sum float64
	invT := 1.0 / temp
	for i, l := range logits {
		p := math.Exp(float64(l-maxLogit) * invT)
		probs[i] = p
		sum += p
	}

	r := rng.Float64() * sum
	var acc float64
	for i, p := range probs {
		acc += p
		if acc >= r {
			return i
		}
	}
	return len(logits) - 1
}

// GenerateText streams generated tokens
func (e *Engine) GenerateText(ctx context.Context, prompt string, maxTokens int, streamingFunc func(ctx context.Context, chunk []byte) error) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.model.reset()

	formattedPrompt := fmt.Sprintf("<|im_start|>system\nYou are a helpful log analysis assistant. Answer accurately and follow the user's language request (e.g. Japanese if requested).<|im_end|>\n<|im_start|>user\n%s<|im_end|>\n<|im_start|>assistant\n", prompt)
	ids := e.tok.Encode(formattedPrompt)
	if len(ids) == 0 {
		ids = e.tok.Encode(prompt)
	}

	// Prefill / feed prompt
	var logits []float32
	for pos, id := range ids {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		logits = e.model.step(id, pos)
	}

	// Decode loop
	var result strings.Builder
	pos := len(ids)
	limit := maxTokens
	if limit <= 0 {
		limit = 512
	}

	for i := 0; i < limit && pos < e.model.cfg.MaxPos-1; i++ {
		select {
		case <-ctx.Done():
			return result.String(), ctx.Err()
		default:
		}

		next := sampleToken(logits, 0.0, 0.9, e.rng)
		if next == e.imEnd || next == e.eot {
			break
		}

		piece := e.tok.Decode([]int{next})
		result.WriteString(piece)

		if streamingFunc != nil {
			if err := streamingFunc(ctx, []byte(piece)); err != nil {
				return result.String(), err
			}
		}

		logits = e.model.step(next, pos)
		pos++
	}

	return result.String(), nil
}
