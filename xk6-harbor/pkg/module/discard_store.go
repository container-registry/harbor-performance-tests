package module

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// discardStore implements oras.Target but only keeps manifest content in memory,
// discarding blob data. Used for pull operations where blob content isn't needed.
type discardStore struct {
	mu       sync.RWMutex
	content  map[digest.Digest][]byte
	refToDesc map[string]ocispec.Descriptor
}

func newDiscardStore() *discardStore {
	return &discardStore{
		content:   make(map[digest.Digest][]byte),
		refToDesc: make(map[string]ocispec.Descriptor),
	}
}

func (ds *discardStore) Exists(ctx context.Context, target ocispec.Descriptor) (bool, error) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	_, ok := ds.content[target.Digest]
	return ok, nil
}

func (ds *discardStore) Fetch(ctx context.Context, target ocispec.Descriptor) (io.ReadCloser, error) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	data, ok := ds.content[target.Digest]
	if !ok {
		return nil, fmt.Errorf("content not found: %s", target.Digest)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (ds *discardStore) Push(ctx context.Context, expected ocispec.Descriptor, r io.Reader) error {
	if isAllowedMediaType(
		expected.MediaType,
		ocispec.MediaTypeImageManifest,
		ocispec.MediaTypeImageIndex,
		"application/vnd.docker.distribution.manifest.v2+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
	) {
		data, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		ds.mu.Lock()
		ds.content[expected.Digest] = data
		ds.mu.Unlock()
	} else {
		// Discard blob data but still drain the reader
		_, _ = io.Copy(io.Discard, r)
	}
	return nil
}

func (ds *discardStore) Resolve(ctx context.Context, ref string) (ocispec.Descriptor, error) {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	desc, ok := ds.refToDesc[ref]
	if !ok {
		return ocispec.Descriptor{}, fmt.Errorf("reference not found: %s", ref)
	}
	return desc, nil
}

func (ds *discardStore) Tag(ctx context.Context, desc ocispec.Descriptor, ref string) error {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.refToDesc[ref] = desc
	return nil
}

func (ds *discardStore) Predecessors(ctx context.Context, node ocispec.Descriptor) ([]ocispec.Descriptor, error) {
	return nil, nil
}

func isAllowedMediaType(mediaType string, allowedMediaTypes ...string) bool {
	if len(allowedMediaTypes) == 0 {
		return true
	}
	for _, allowedMediaType := range allowedMediaTypes {
		if mediaType == allowedMediaType {
			return true
		}
	}
	return false
}
