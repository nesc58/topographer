package providers

import (
	"bufio"
	"encoding/hex"
	"errors"
	"hash"
	"io"
)

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
