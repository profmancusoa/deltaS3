// Command deltaS3 exposes the user-facing CLI for local chunking, S3 pushes,
// and local restore operations.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"deltas3/internal/backup"
	"deltas3/internal/restore"
	"deltas3/internal/upload"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "chunk":
		runChunk(os.Args[2:])
	case "push":
		runPush(os.Args[2:])
	case "restore":
		runRestore(os.Args[2:])
	case "version":
		runVersion()
	default:
		usage()
		os.Exit(2)
	}
}

func runChunk(args []string) {
	var cfg backup.Config

	fs := flag.NewFlagSet("chunk", flag.ExitOnError)
	fs.StringVar(&cfg.InputPath, "in", "", "path to the local file to chunk")
	fs.StringVar(&cfg.OutputDir, "out-dir", "", "directory where chunks and manifest will be written")
	fs.Int64Var(&cfg.ChunkSize, "chunk-size", 8*1024*1024, "chunk size in bytes")
	fs.StringVar(&cfg.Bucket, "bucket", "", "bucket name to embed in the manifest")
	fs.StringVar(&cfg.Prefix, "prefix", "", "object prefix to embed in the manifest")
	fs.StringVar(&cfg.LogicalName, "name", "", "logical artifact name stored in the manifest")
	fs.Parse(args)

	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid chunk configuration: %v", err)
	}

	result, err := backup.Run(cfg)
	if err != nil {
		log.Fatalf("chunk failed: %v", err)
	}

	fmt.Fprintf(os.Stdout, "manifest: %s\n", result.ManifestPath)
	fmt.Fprintf(os.Stdout, "chunks written: %d\n", result.ChunksWritten)
	fmt.Fprintf(os.Stdout, "chunks reused: %d\n", result.ChunksReused)
	fmt.Fprintf(os.Stdout, "artifact hash: %s\n", result.ArtifactHash)
}

func runRestore(args []string) {
	var cfg restore.Config

	fs := flag.NewFlagSet("restore", flag.ExitOnError)
	fs.StringVar(&cfg.ManifestPath, "manifest", "", "path to the manifest.json file")
	fs.StringVar(&cfg.ChunksDir, "chunks", "", "directory containing chunk files")
	fs.StringVar(&cfg.OutputPath, "out", "", "path of the reconstructed file")
	fs.Parse(args)

	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid restore configuration: %v", err)
	}

	result, err := restore.Run(cfg)
	if err != nil {
		log.Fatalf("restore failed: %v", err)
	}

	fmt.Fprintf(os.Stdout, "output: %s\n", result.OutputPath)
	fmt.Fprintf(os.Stdout, "chunks restored: %d\n", result.ChunksRestored)
	fmt.Fprintf(os.Stdout, "artifact hash: %s\n", result.ArtifactHash)
}

func runPush(args []string) {
	var cfg upload.Config

	fs := flag.NewFlagSet("push", flag.ExitOnError)
	fs.StringVar(&cfg.ManifestPath, "manifest", "", "path to the manifest.json file")
	fs.StringVar(&cfg.ChunksDir, "chunks", "", "directory containing chunk files")
	fs.StringVar(&cfg.S3ConfigPath, "config", "", "path to the S3 configuration JSON file")
	fs.Parse(args)

	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid push configuration: %v", err)
	}

	result, err := upload.Run(cfg)
	if err != nil {
		log.Fatalf("push failed: %v", err)
	}

	fmt.Fprintf(os.Stdout, "bucket: %s\n", result.Bucket)
	fmt.Fprintf(os.Stdout, "prefix: %s\n", result.Prefix)
	fmt.Fprintf(os.Stdout, "unique chunks uploaded: %d\n", result.ChunksUploaded)
	fmt.Fprintf(os.Stdout, "unique chunks reused: %d\n", result.ChunksReused)
	fmt.Fprintf(os.Stdout, "versioned manifest: %s\n", result.VersionedManifestKey)
	fmt.Fprintf(os.Stdout, "latest manifest: %s\n", result.LatestManifestKey)
}

func runVersion() {
	fmt.Fprintf(os.Stdout, "deltaS3 version %s\n", version)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  deltaS3 chunk   -in <file> -out-dir <dir> [-chunk-size bytes] [-bucket name] [-prefix key] [-name name]")
	fmt.Fprintln(os.Stderr, "  deltaS3 push    -manifest <file> -chunks <dir> -config <file>")
	fmt.Fprintln(os.Stderr, "  deltaS3 restore -manifest <file> -chunks <dir> -out <file>")
	fmt.Fprintln(os.Stderr, "  deltaS3 version")
}
