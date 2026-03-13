package scenarios

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync/atomic"
	"time"

	"github.com/goharbor/perf/pkg/config"
	"github.com/goharbor/perf/pkg/harbor"
	"github.com/goharbor/perf/pkg/runner"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	log "github.com/sirupsen/logrus"
)

type PushSameProject struct {
	cfg *config.Config
}

type pushSameProjectData struct {
	store   *harbor.ContentStore
	blobsArr [][]*ocispec.Descriptor
	refs    []string
	counter atomic.Int64
}

func (s *PushSameProject) Name() string { return "push-artifacts-to-same-projects" }

func (s *PushSameProject) Setup(_ context.Context, _ *harbor.Client) (runner.SharedData, error) {
	projectName := harbor.RandomItem(harbor.GetProjectNames(s.cfg))
	repositoryName := fmt.Sprintf("repository-%d", time.Now().UnixMilli())

	storePath := filepath.Join(os.TempDir(), "harbor", fmt.Sprintf("push-same-%d", time.Now().UnixNano()))
	store, err := harbor.NewContentStore(storePath)
	if err != nil {
		return nil, fmt.Errorf("create content store: %w", err)
	}

	iterations := s.cfg.Iterations
	blobsArr := make([][]*ocispec.Descriptor, iterations)
	refs := make([]string, iterations)

	for i := 0; i < iterations; i++ {
		blobs, err := store.GenerateMany(s.cfg.BlobSize, s.cfg.BlobsCountPerArtifact)
		if err != nil {
			return nil, fmt.Errorf("generate blobs: %w", err)
		}
		blobsArr[i] = blobs
		refs[i] = fmt.Sprintf("%s/%s:tag-%s", projectName, repositoryName,
			harbor.NumberToPadString(i, iterations))
	}

	return &pushSameProjectData{
		store:    store,
		blobsArr: blobsArr,
		refs:     refs,
	}, nil
}

func (s *PushSameProject) InitWorker(_ context.Context, _ *harbor.Client, _ runner.SharedData) (runner.WorkerState, error) {
	return nil, nil
}

func (s *PushSameProject) Run(ctx context.Context, h *harbor.Client, data runner.SharedData, _ runner.WorkerState) error {
	d := data.(*pushSameProjectData)
	i := int(d.counter.Add(1)) - 1
	if i >= len(d.refs) {
		return fmt.Errorf("iteration index %d out of range", i)
	}

	_, err := h.Push(ctx, harbor.PushOption{
		Ref:   d.refs[i],
		Store: d.store,
		Blobs: derefDescs(d.blobsArr[i]),
	})
	return err
}

func (s *PushSameProject) Teardown(ctx context.Context, h *harbor.Client, data runner.SharedData) error {
	d := data.(*pushSameProjectData)
	_ = d.store.Free()

	re := regexp.MustCompile(`([^/]+)/([^:]+):(.*)`)
	for _, ref := range d.refs {
		m := re.FindStringSubmatch(ref)
		if len(m) == 4 {
			if err := h.DeleteArtifact(ctx, m[1], m[2], m[3]); err != nil {
				log.Debugf("cleanup %s: %v", ref, err)
			}
		}
	}
	return nil
}

// derefDescs converts []*ocispec.Descriptor to []ocispec.Descriptor.
func derefDescs(ptrs []*ocispec.Descriptor) []ocispec.Descriptor {
	result := make([]ocispec.Descriptor, len(ptrs))
	for i, p := range ptrs {
		result[i] = *p
	}
	return result
}
