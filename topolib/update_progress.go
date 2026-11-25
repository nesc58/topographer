package topolib

import "context"

// UpdateProgressLogger receives detailed events during an offline provider update.
type UpdateProgressLogger interface {
	UpdateProgress(provider, step string, details map[string]string)
}

type updateProgressContextKey struct{}

type updateProgressContext struct {
	provider string
	logger   UpdateProgressLogger
}

// WithUpdateProgressLogger adds an update progress logger to ctx.
func WithUpdateProgressLogger(ctx context.Context, provider string, logger UpdateProgressLogger) context.Context {
	return context.WithValue(ctx, updateProgressContextKey{}, updateProgressContext{
		provider: provider,
		logger:   logger,
	})
}

// LogUpdateProgress reports a detailed update event when ctx has a progress logger.
func LogUpdateProgress(ctx context.Context, step string, details map[string]string) {
	progress, ok := ctx.Value(updateProgressContextKey{}).(updateProgressContext)
	if !ok || progress.logger == nil {
		return
	}

	progress.logger.UpdateProgress(progress.provider, step, details)
}
