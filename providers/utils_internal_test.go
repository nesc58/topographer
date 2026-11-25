package providers

import (
	"context"
	"testing"

	"github.com/nesc58/topographer/topolib"
)

type updateProgressRecorder struct {
	provider string
	step     string
	details  map[string]string
}

func (r *updateProgressRecorder) UpdateProgress(provider, step string, details map[string]string) {
	r.provider = provider
	r.step = step
	r.details = details
}

func TestLogDownloadSourceRedactsCredentials(t *testing.T) {
	recorder := &updateProgressRecorder{}
	ctx := topolib.WithUpdateProgressLogger(context.Background(), "ip2location", recorder)

	logDownloadSource(ctx, "https://user:password@example.com/download?token=secret", "database archive")

	if recorder.provider != "ip2location" || recorder.step != "download_source" {
		t.Fatalf("unexpected progress event: provider=%q step=%q", recorder.provider, recorder.step)
	}
	if got, want := recorder.details["url"], "https://example.com/download"; got != want {
		t.Errorf("logged URL = %q, want %q", got, want)
	}
	if got, want := recorder.details["artifact"], "database archive"; got != want {
		t.Errorf("logged artifact = %q, want %q", got, want)
	}
}
