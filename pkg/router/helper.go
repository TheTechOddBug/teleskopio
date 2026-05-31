package router

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"helm.sh/helm/v3/pkg/release"
)

func decodeHelmRelease(data []byte) (release.Release, error) {
	var release release.Release
	decodedBytes, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return release, fmt.Errorf("decoding string: %w", err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(decodedBytes))
	if err != nil {
		return release, fmt.Errorf("creating gzip reader: %w", err)
	}
	defer gz.Close()

	decoded, err := io.ReadAll(gz)
	if err != nil {
		return release, fmt.Errorf("decompressing data: %w", err)
	}

	if err := json.Unmarshal(decoded, &release); err != nil {
		return release, fmt.Errorf("unmarshalling JSON: %w", err)
	}
	return release, nil
}
