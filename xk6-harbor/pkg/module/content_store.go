package module

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/dustin/go-humanize"
	"github.com/goharbor/xk6-harbor/pkg/util"
	"github.com/grafana/sobek"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	ants "github.com/panjf2000/ants/v2"
)

// casStore is a content-addressable file store that derives paths from digests.
// Unlike oras-go/v2's file.Store, it has no in-memory index, so it survives
// k6's SharedArray/VU phase boundary.
type casStore struct {
	rootPath string
}

func newCASStore(rootPath string) (*casStore, error) {
	if err := os.MkdirAll(rootPath, 0755); err != nil {
		return nil, err
	}
	return &casStore{rootPath: rootPath}, nil
}

func (s *casStore) Push(ctx context.Context, expected ocispec.Descriptor, r io.Reader) error {
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

func (s *casStore) Fetch(ctx context.Context, target ocispec.Descriptor) (io.ReadCloser, error) {
	p := filepath.Join(s.rootPath, "blobs", target.Digest.Algorithm().String(), target.Digest.Encoded())
	return os.Open(p)
}

func newContentStore(rt *sobek.Runtime, name string) *ContentStore {
	rootPath := filepath.Join(DefaultRootPath, name)

	store, err := newCASStore(rootPath)
	Check(rt, err)

	return &ContentStore{Runtime: rt, Store: store, RootPath: rootPath}
}

type ContentStore struct {
	Runtime  *sobek.Runtime
	Store    *casStore
	RootPath string
}

func (s *ContentStore) Generate(humanSize sobek.Value) (*ocispec.Descriptor, error) {
	size, err := humanize.ParseBytes(humanSize.String())
	if err != nil {
		return nil, err
	}

	data, err := util.GenerateRandomBytes(int(size))
	if err != nil {
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

	err = s.Store.Push(context.Background(), desc, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	return &desc, nil
}

func (s *ContentStore) GenerateMany(humanSize sobek.Value, count int) ([]*ocispec.Descriptor, error) {
	size, err := humanize.ParseBytes(humanSize.String())
	if err != nil {
		return nil, err
	}
	if size == 0 {
		return nil, errors.New("size must bigger than zero")
	}

	if count <= 0 {
		return nil, errors.New("count must bigger than zero")
	}

	descriptors := make([]*ocispec.Descriptor, count)
	errs := make([]error, count)

	var wg sync.WaitGroup

	poolSize := DefaultPoolSise
	if count < poolSize {
		poolSize = count
	}

	p, _ := ants.NewPoolWithFunc(poolSize, func(i interface{}) {
		defer wg.Done()

		ix := i.(int)
		descriptor, err := s.Generate(humanSize)
		if err != nil {
			errs[ix] = err
		} else {
			descriptors[ix] = descriptor
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

	return descriptors, nil
}

func (s *ContentStore) Free() {
	err := os.RemoveAll(s.RootPath)
	if err != nil {
		panic(s.Runtime.NewGoError(err))
	}
}
