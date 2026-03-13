package harbor

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/dustin/go-humanize"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	ants "github.com/panjf2000/ants/v2"
)

const defaultPoolSize = 300

// casStore is a content-addressable file store that derives paths from digests.
type casStore struct {
	rootPath string
}

func newCASStore(rootPath string) (*casStore, error) {
	if err := os.MkdirAll(rootPath, 0755); err != nil {
		return nil, err
	}
	return &casStore{rootPath: rootPath}, nil
}

func (s *casStore) Push(_ context.Context, expected ocispec.Descriptor, r io.Reader) error {
	dir := filepath.Join(s.rootPath, "blobs", expected.Digest.Algorithm().String())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, expected.Digest.Encoded()), data, 0664)
}

func (s *casStore) Fetch(_ context.Context, target ocispec.Descriptor) (io.ReadCloser, error) {
	p := filepath.Join(s.rootPath, "blobs", target.Digest.Algorithm().String(), target.Digest.Encoded())
	return os.Open(p)
}

// ContentStore manages content-addressable blob storage for OCI artifacts.
type ContentStore struct {
	Store    *casStore
	RootPath string
}

// NewContentStore creates a new content store at the given path.
func NewContentStore(rootPath string) (*ContentStore, error) {
	store, err := newCASStore(rootPath)
	if err != nil {
		return nil, err
	}
	return &ContentStore{Store: store, RootPath: rootPath}, nil
}

// Generate creates a single random blob of the given human-readable size.
func (s *ContentStore) Generate(humanSize string) (*ocispec.Descriptor, error) {
	size, err := humanize.ParseBytes(humanSize)
	if err != nil {
		return nil, err
	}

	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return nil, err
	}

	dgt := digest.FromBytes(data)
	desc := ocispec.Descriptor{
		MediaType: "k6-x-harbor",
		Digest:    dgt,
		Size:      int64(len(data)),
		Annotations: map[string]string{
			ocispec.AnnotationTitle: "raw",
		},
	}

	if err := s.Store.Push(context.Background(), desc, bytes.NewReader(data)); err != nil {
		return nil, err
	}

	return &desc, nil
}

// GenerateMany creates multiple random blobs in parallel.
func (s *ContentStore) GenerateMany(humanSize string, count int) ([]*ocispec.Descriptor, error) {
	size, err := humanize.ParseBytes(humanSize)
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, errors.New("size must be greater than zero")
	}
	if count <= 0 {
		return nil, errors.New("count must be greater than zero")
	}

	descriptors := make([]*ocispec.Descriptor, count)
	errs := make([]error, count)

	var wg sync.WaitGroup

	poolSize := defaultPoolSize
	if count < poolSize {
		poolSize = count
	}

	p, _ := ants.NewPoolWithFunc(poolSize, func(i interface{}) {
		defer wg.Done()
		ix := i.(int)
		desc, err := s.Generate(humanSize)
		if err != nil {
			errs[ix] = err
		} else {
			descriptors[ix] = desc
		}
	})
	defer p.Release()

	for i := 0; i < count; i++ {
		wg.Add(1)
		_ = p.Invoke(i)
	}

	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	_ = size // suppress unused warning, already parsed and used by Generate
	return descriptors, nil
}

// Free removes all stored content.
func (s *ContentStore) Free() error {
	return os.RemoveAll(s.RootPath)
}
