// Package embedding runs EmbeddingGemma in-process on the CPU with ONNX
// Runtime, so recipe embeddings need no external provider.
package embedding

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sync"

	ort "github.com/yalue/onnxruntime_go"
)

// Model names the embeddings this package produces; stored embeddings from
// any other model are replaced on the next sync.
const Model = "embeddinggemma-300m-onnx-q8"

// Files expected in the model directory (from
// onnx-community/embeddinggemma-300m-ONNX; the Dockerfile downloads them).
const (
	modelFile     = "model_quantized.onnx" // int8: same benchmark scores as fp32 at a quarter of the size
	tokenizerFile = "tokenizer.json"
)

const (
	maxTokens = 2048 // the model's context; recipe texts use ~100
	batchSize = 16   // texts of similar length run together to limit padding
)

// Embedder loads the model for each Embed call and frees it afterwards:
// syncs are rare, loading takes about a second, and the ~1 GB it uses is
// better left to the rest of the server in between.
type Embedder struct {
	dir string
	lib string

	mu sync.Mutex // one Embed at a time
}

// The ONNX Runtime environment is process-wide: it is set up once, from the
// first embedder used.
var (
	ortOnce sync.Once
	ortErr  error
)

// New returns an embedder for the model files in dir, using the ONNX Runtime
// shared library at lib.
func New(dir, lib string) *Embedder {
	return &Embedder{dir: dir, lib: lib}
}

// Available reports whether the model files and runtime library exist.
func (e *Embedder) Available() bool {
	for _, p := range []string{filepath.Join(e.dir, modelFile), filepath.Join(e.dir, tokenizerFile), e.lib} {
		if _, err := os.Stat(p); err != nil {
			return false
		}
	}
	return true
}

func (e *Embedder) init() error {
	ortOnce.Do(func() {
		ort.SetSharedLibraryPath(e.lib)
		ortErr = ort.InitializeEnvironment()
	})
	return ortErr
}

// Embed returns one unit-length vector per text, in input order.
func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.init(); err != nil {
		return nil, fmt.Errorf("onnxruntime: %w", err)
	}
	tok, err := loadTokenizer(filepath.Join(e.dir, tokenizerFile))
	if err != nil {
		return nil, err
	}
	session, err := ort.NewDynamicAdvancedSession(filepath.Join(e.dir, modelFile),
		[]string{"input_ids", "attention_mask"}, []string{"sentence_embedding"}, nil)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", modelFile, err)
	}
	defer destroy(session)

	ids := make([][]int32, len(texts))
	order := make([]int, len(texts))
	for i, t := range texts {
		ids[i] = tok.encode(t, maxTokens)
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return len(ids[a]) - len(ids[b]) })

	out := make([][]float32, len(texts))
	for start := 0; start < len(order); start += batchSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		batch := order[start:min(start+batchSize, len(order))]
		vecs, err := run(session, ids, batch)
		if err != nil {
			return nil, err
		}
		for k, i := range batch {
			out[i] = vecs[k]
		}
	}
	return out, nil
}

// run embeds one batch, right-padding every sequence to the longest.
func run(session *ort.DynamicAdvancedSession, ids [][]int32, batch []int) ([][]float32, error) {
	width := 0
	for _, i := range batch {
		width = max(width, len(ids[i]))
	}
	inputIDs := make([]int64, len(batch)*width)
	mask := make([]int64, len(batch)*width)
	for k, i := range batch {
		for j, id := range ids[i] {
			inputIDs[k*width+j] = int64(id)
			mask[k*width+j] = 1
		}
	}
	shape := ort.NewShape(int64(len(batch)), int64(width))
	idTensor, err := ort.NewTensor(shape, inputIDs)
	if err != nil {
		return nil, err
	}
	defer destroy(idTensor)
	maskTensor, err := ort.NewTensor(shape, mask)
	if err != nil {
		return nil, err
	}
	defer destroy(maskTensor)

	outputs := []ort.Value{nil}
	if err := session.Run([]ort.Value{idTensor, maskTensor}, outputs); err != nil {
		return nil, fmt.Errorf("embedding run: %w", err)
	}
	defer destroy(outputs[0])
	result, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, errors.New("embedding run: output is not a float32 tensor")
	}
	data := result.GetData()
	dims := len(data) / len(batch)
	vecs := make([][]float32, len(batch))
	for k := range batch {
		v := slices.Clone(data[k*dims : (k+1)*dims])
		var norm float64
		for _, x := range v {
			norm += float64(x) * float64(x)
		}
		norm = math.Sqrt(norm)
		if norm == 0 {
			return nil, errors.New("embedding run: zero vector")
		}
		for j := range v {
			v[j] = float32(float64(v[j]) / norm)
		}
		vecs[k] = v
	}
	return vecs, nil
}

// destroy frees an ONNX Runtime object; a failure there leaves nothing to
// recover.
func destroy(o interface{ Destroy() error }) { _ = o.Destroy() }
