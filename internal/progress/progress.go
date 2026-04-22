// Package progress provides lightweight terminal progress reporting shared by
// long-running commands such as chunk creation and S3 uploads.
package progress

import (
	"fmt"
	"io"
	"math"
	"sync"
	"time"
)

const minRenderInterval = 200 * time.Millisecond

type Reporter struct {
	label      string
	totalBytes int64
	writer     io.Writer

	mu         sync.Mutex
	lastRender time.Time
	finished   bool
}

func New(label string, totalBytes int64, writer io.Writer) *Reporter {
	return &Reporter{
		label:      label,
		totalBytes: totalBytes,
		writer:     writer,
	}
}

func (r *Reporter) Update(doneBytes int64, detail string) {
	r.render(doneBytes, detail, false)
}

func (r *Reporter) Finish(doneBytes int64, detail string) {
	r.render(doneBytes, detail, true)
}

func (r *Reporter) render(doneBytes int64, detail string, force bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.finished {
		return
	}

	now := time.Now()
	// Throttle terminal updates to keep progress rendering readable and avoid
	// spending too much time repainting stderr on large runs.
	if !force && !r.lastRender.IsZero() && now.Sub(r.lastRender) < minRenderInterval {
		return
	}

	line := fmt.Sprintf(
		"\r%s: %s / %s (%5.1f%%)%s",
		r.label,
		formatBytes(doneBytes),
		formatBytes(r.totalBytes),
		percent(doneBytes, r.totalBytes),
		renderDetail(detail),
	)
	fmt.Fprint(r.writer, line)
	r.lastRender = now

	if force {
		fmt.Fprint(r.writer, "\n")
		r.finished = true
	}
}

func percent(doneBytes, totalBytes int64) float64 {
	if totalBytes <= 0 {
		if doneBytes > 0 {
			return 100
		}
		return 0
	}

	p := (float64(doneBytes) / float64(totalBytes)) * 100
	return math.Min(p, 100)
}

func renderDetail(detail string) string {
	if detail == "" {
		return ""
	}

	return " | " + detail
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}

	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
