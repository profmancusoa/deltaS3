# deltaS3

deltaS3 is a file-oriented backup tool designed to minimize WAN traffic when publishing large artifacts to Amazon S3 or S3-compatible object storage.

The core idea is simple:

- split a large local file into fixed-size chunks
- identify each chunk by its content hash
- upload only the chunks that are not already present remotely
- store a manifest that describes how to reconstruct the original file

This makes deltaS3 suitable for disaster recovery workflows where local artifacts are large, changes between runs are partial, and restore operations are relatively rare.

Release history is tracked in [CHANGELOG.md](/run/host/home/mancusoa/Projects/deltaS3/CHANGELOG.md:1).

## Architecture

deltaS3 is intentionally built around a small set of concepts:

- `artifact`: the original local file to protect
- `chunk`: a fixed-size slice of the artifact
- `manifest`: the metadata document that describes the full artifact and its chunk layout
- `object store`: the S3 bucket/prefix where chunks and manifests are published

The current workflow is:

1. `chunk`
   Reads a local file sequentially, splits it into fixed-size chunks, hashes each chunk with `BLAKE3`, writes the chunks locally, and generates `manifest.json`.

2. `push`
   Reads the local manifest, checks which referenced chunks already exist on S3, uploads only missing chunks, then publishes both:
   - `manifests/<backup_id>.json`
   - `manifests/latest.json`

3. `restore`
   Reads a manifest plus a local chunk directory, reconstructs the original file, verifies each chunk hash, and verifies the final artifact hash.

### Important design choices

- deltaS3 is generic and works on a normal local file. It does not depend on Proxmox, ZFS, VM formats, or any specific backup producer.
- Chunks are currently fixed-size. This keeps the implementation simple and deterministic.
- Chunk identity is based on `BLAKE3`, chosen as a strong and fast content hash.
- Each manifest describes a complete logical artifact, not a delta chain.
- Restore is atomic at file level: the output is first written to a temporary `.partial` file and promoted only after hash verification succeeds.
- The current S3 layout is:

```text
<prefix>/
  manifests/
    latest.json
    <backup_id>.json
  chunks/
    <chunk-hash>
```

### Current scope

The current implementation already supports:

- local chunk generation
- local restore from manifest + chunks
- push of chunks and manifests to S3
- visual progress feedback for `chunk` and `push`

Remote retention and garbage collection of old manifests/chunks are planned as incremental next steps.

## Configuration

The `push` command reads S3 credentials and connection settings from an external JSON file.

Example:

```json
{
  "region": "eu-west-1",
  "endpoint": "https://s3.eu-west-1.amazonaws.com",
  "force_path_style": false,
  "credentials": {
    "access_key_id": "AKIA...",
    "secret_access_key": "SECRET...",
    "session_token": ""
  }
}
```

Fields:

- `region`: AWS region or logical region required by the target S3 API
- `endpoint`: optional custom endpoint; useful for S3-compatible storage
- `force_path_style`: forces path-style S3 URLs; often useful with non-AWS providers
- `credentials.access_key_id`: access key
- `credentials.secret_access_key`: secret key
- `credentials.session_token`: optional session token for temporary credentials

Notes:

- For AWS S3, `endpoint` can usually be omitted and `force_path_style` should typically stay `false`.
- For MinIO or some S3-compatible providers, `endpoint` is usually required and `force_path_style` is often better set to `true`.

## Usage

Build the binary:

```bash
make build
```

This produces:

```text
bin/deltaS3
```

You can print the current binary version with:

```bash
./bin/deltaS3 version
```

The build version is defined in the `Makefile`:

```bash
VERSION := 0.1.0
```

### Chunk a local file

```bash
./bin/deltaS3 chunk \
  -in /path/to/file.bin \
  -out-dir /tmp/deltaS3-out \
  -chunk-size 8388608 \
  -bucket my-bucket \
  -prefix backups/file-bin \
  -name file.bin
```

Output:

- `/tmp/deltaS3-out/chunks/<hash>`
- `/tmp/deltaS3-out/manifest.json`

Arguments:

- `-in`: source file to process
- `-out-dir`: local directory used to store generated chunks and manifest
- `-chunk-size`: chunk size in bytes
- `-bucket`: bucket name embedded into the manifest
- `-prefix`: remote object prefix embedded into the manifest
- `-name`: optional logical artifact name stored in the manifest

### Push chunks and manifest to S3

```bash
./bin/deltaS3 push \
  -manifest /tmp/deltaS3-out/manifest.json \
  -chunks /tmp/deltaS3-out/chunks \
  -config /path/to/s3-config.json
```

Behavior:

- checks whether each unique chunk already exists remotely
- uploads only missing chunks
- writes a versioned manifest to `manifests/<backup_id>.json`
- updates `manifests/latest.json`

### Restore a file from local chunks

```bash
./bin/deltaS3 restore \
  -manifest /tmp/deltaS3-out/manifest.json \
  -chunks /tmp/deltaS3-out/chunks \
  -out /tmp/restored.bin
```

Behavior:

- reconstructs the file in chunk order
- verifies every chunk hash during reconstruction
- verifies the final artifact hash
- publishes the final file only after successful verification

### Smoke test

You can run the built-in smoke test with:

```bash
make smoke-test
```

This performs a local end-to-end validation of:

- `chunk`
- `restore`
- final file equality check
