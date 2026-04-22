// Package restore rebuilds an artifact from a manifest and a local chunk
// directory while verifying chunk and artifact integrity.
package restore

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/zeebo/blake3"

	"deltas3/internal/manifest"
)

type Config struct {
	ManifestPath string
	ChunksDir    string
	OutputPath   string
}

type Result struct {
	OutputPath     string
	ArtifactHash   string
	ChunksRestored int
}

func (c Config) Validate() error {
	if c.ManifestPath == "" {
		return fmt.Errorf("manifest is required")
	}
	if c.ChunksDir == "" {
		return fmt.Errorf("chunks-dir is required")
	}
	if c.OutputPath == "" {
		return fmt.Errorf("output is required")
	}

	return nil
}

func Run(cfg Config) (Result, error) {
	doc, err := manifest.LoadFile(cfg.ManifestPath)
	if err != nil {
		return Result{}, fmt.Errorf("load manifest: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(cfg.OutputPath), 0o755); err != nil {
		return Result{}, fmt.Errorf("create output dir: %w", err)
	}

	// Restore into a temporary file first so a failed run never leaves behind a
	// partially reconstructed artifact at the final path.
	tempPath := cfg.OutputPath + ".partial"
	out, err := os.Create(tempPath)
	if err != nil {
		return Result{}, fmt.Errorf("create output: %w", err)
	}

	cleanupTemp := true
	defer func() {
		_ = out.Close()
		if cleanupTemp {
			_ = os.Remove(tempPath)
		}
	}()

	artifactHasher := blake3.New()

	for _, chunk := range doc.Chunks {
		chunkPath := filepath.Join(cfg.ChunksDir, chunk.Hash.Value)
		if err := restoreChunk(out, artifactHasher, chunkPath, chunk); err != nil {
			return Result{}, err
		}
	}

	if err := out.Close(); err != nil {
		return Result{}, fmt.Errorf("close output: %w", err)
	}

	artifactHash := hex.EncodeToString(artifactHasher.Sum(nil))
	if artifactHash != doc.Artifact.Hash.Value {
		return Result{}, fmt.Errorf("artifact hash mismatch: got %s want %s", artifactHash, doc.Artifact.Hash.Value)
	}

	if err := os.Rename(tempPath, cfg.OutputPath); err != nil {
		return Result{}, fmt.Errorf("publish output: %w", err)
	}
	cleanupTemp = false

	return Result{
		OutputPath:     cfg.OutputPath,
		ArtifactHash:   artifactHash,
		ChunksRestored: len(doc.Chunks),
	}, nil
}

func restoreChunk(out *os.File, artifactHasher *blake3.Hasher, chunkPath string, meta manifest.Chunk) error {
	in, err := os.Open(chunkPath)
	if err != nil {
		return fmt.Errorf("open chunk %s: %w", meta.Hash.Value, err)
	}
	defer in.Close()

	chunkHasher := blake3.New()
	// Hash the chunk while streaming it into the output file so restore stays
	// sequential and avoids loading the whole chunk into memory.
	written, err := io.Copy(io.MultiWriter(out, artifactHasher, chunkHasher), in)
	if err != nil {
		return fmt.Errorf("copy chunk %s: %w", meta.Hash.Value, err)
	}
	if written != meta.SizeBytes {
		return fmt.Errorf("chunk %s size mismatch: got %d want %d", meta.Hash.Value, written, meta.SizeBytes)
	}

	chunkHash := hex.EncodeToString(chunkHasher.Sum(nil))
	if chunkHash != meta.Hash.Value {
		return fmt.Errorf("chunk %s hash mismatch: got %s want %s", meta.Hash.Value, chunkHash, meta.Hash.Value)
	}

	return nil
}
