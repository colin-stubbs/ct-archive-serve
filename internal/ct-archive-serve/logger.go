package ctarchiveserve

import (
	"context"
	"io"
	"log/slog"
	"os"
)

// LoggerOptions controls structured logging behavior.
type LoggerOptions struct {
	Verbose bool
	Debug   bool
}

// NewLogger constructs a structured JSON logger that writes INFO/WARN/DEBUG to stdout
// and ERROR+ to stderr, per specs/001-ct-archive-serve/spec.md (NFR-010).
//
// Verbose is used by request logging (not by slog filtering); Debug enables slog DEBUG logs.
func NewLogger(opts LoggerOptions) *slog.Logger {
	level := slog.LevelInfo
	if opts.Debug {
		level = slog.LevelDebug
	}

	outHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	errHandler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})

	return slog.New(&splitLevelHandler{
		stdout: outHandler,
		stderr: errHandler,
	})
}

type splitLevelHandler struct {
	stdout slog.Handler
	stderr slog.Handler
}

func (h *splitLevelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	if level >= slog.LevelError {
		return h.stderr.Enabled(ctx, level)
	}
	return h.stdout.Enabled(ctx, level)
}

func (h *splitLevelHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelError {
		return h.stderr.Handle(ctx, r)
	}
	return h.stdout.Handle(ctx, r)
}

func (h *splitLevelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &splitLevelHandler{
		stdout: h.stdout.WithAttrs(attrs),
		stderr: h.stderr.WithAttrs(attrs),
	}
}

func (h *splitLevelHandler) WithGroup(name string) slog.Handler {
	return &splitLevelHandler{
		stdout: h.stdout.WithGroup(name),
		stderr: h.stderr.WithGroup(name),
	}
}

var _ slog.Handler = (*splitLevelHandler)(nil)

// discardWriter is used in tests when a writer is required.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

var _ io.Writer = discardWriter{}

