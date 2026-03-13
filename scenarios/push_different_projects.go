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

type PushDifferentProjects struct {
	cfg *config.Config
}

type pushDiffProjectsData struct {
	store    *harbor.ContentStore
	blobsArr [][]*ocispec.Descriptor
	refs     []string
	counter  atomic.Int64
}

func (s *PushDifferentProjects) Name() string { return "push-artifacts-to-different-projects" }

func (s *PushDifferentProjects) Setup(_ context.Context, _ *harbor.Client) (runner.SharedData, error) {
	projectNames := harbor.GetProjectNames(s.cfg)

	storePath := filepath.Join(os.TempDir(), "harbor", fmt.Sprintf("push-diff-%d", time.Now().UnixNano()))
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
		refs[i] = fmt.Sprintf("%s/repository-%d:tag-%s",
			harbor.RandomItem(projectNames),
			time.Now().UnixMilli(),
			harbor.NumberToPadString(i, iterations))
	}

	return &pushDiffProjectsData{
		store:    store,
		blobsArr: blobsArr,
		refs:     refs,
	}, nil
}

func (s *PushDifferentProjects) InitWorker(_ context.Context, _ *harbor.Client, _ runner.SharedData) (runner.WorkerState, error) {
	return nil, nil
}

func (s *PushDifferentProjects) Run(ctx context.Context, h *harbor.Client, data runner.SharedData, _ runner.WorkerState) error {
	d := data.(*pushDiffProjectsData)
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

func (s *PushDifferentProjects) Teardown(ctx context.Context, h *harbor.Client, data runner.SharedData) error {
	d := data.(*pushDiffProjectsData)
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
