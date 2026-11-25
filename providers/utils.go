package providers

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"net/url"

	"github.com/nesc58/topographer/topolib"
)

type providerUpdateProgress struct {
	provider string
	logger   topolib.UpdateProgressLogger
}

func (p providerUpdateProgress) log(step string, details map[string]string) {
	if p.logger != nil {
		p.logger.UpdateProgress(p.provider, step, details)
	}
}

func logDownloadSource(ctx context.Context, rawURL, artifact string) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		topolib.LogUpdateProgress(ctx, "download_source", map[string]string{
			"url":      "unavailable",
			"artifact": artifact,
		})

		return
	}

	parsedURL.User = nil
	parsedURL.RawQuery = ""
	parsedURL.ForceQuery = false
	parsedURL.Fragment = ""
	topolib.LogUpdateProgress(ctx, "download_source", map[string]string{
		"url":      parsedURL.String(),
		"artifact": artifact,
	})
}

func logExtraction(ctx context.Context, source, target string) {
	topolib.LogUpdateProgress(ctx, "extracting", map[string]string{
		"source": source,
		"target": target,
	})
}

func flushResponse(resp io.ReadCloser) error {
	_, copyErr := io.Copy(io.Discard, resp)

	return errors.Join(copyErr, resp.Close())
}

func copyResponse(dst io.Writer, src io.Reader) error {
	_, err := io.Copy(dst, bufio.NewReader(src))

	return err
}

func hashedCopyResponse(hashFunc func() hash.Hash, dst io.Writer, src io.Reader) (string, error) {
	hasher := hashFunc()
	err := copyResponse(io.MultiWriter(hasher, dst), src)

	if err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
