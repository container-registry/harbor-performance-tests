package scenarios

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/goharbor/perf/pkg/config"
	"github.com/goharbor/perf/pkg/harbor"
	"github.com/goharbor/perf/pkg/runner"
	log "github.com/sirupsen/logrus"
)

type PullDifferentProjects struct {
	cfg *config.Config
}

type pullDiffProjectsData struct {
	projectNames   []string
	repositoryName string
	reference      string
}

func (s *PullDifferentProjects) Name() string { return "pull-artifacts-from-different-projects" }

func (s *PullDifferentProjects) Setup(ctx context.Context, h *harbor.Client) (runner.SharedData, error) {
	projectNames := harbor.GetProjectNames(s.cfg)
	repositoryName := fmt.Sprintf("repository-%d", time.Now().UnixMilli())
	reference := fmt.Sprintf("tag-%d", time.Now().UnixMilli())

	storePath := filepath.Join(os.TempDir(), "harbor", fmt.Sprintf("pull-diff-%d", time.Now().UnixNano()))
	store, err := harbor.NewContentStore(storePath)
	if err != nil {
		return nil, fmt.Errorf("create content store: %w", err)
	}
	defer store.Free()

	for _, projectName := range projectNames {
		blobs, err := store.GenerateMany(s.cfg.BlobSize, s.cfg.BlobsCountPerArtifact)
		if err != nil {
			return nil, fmt.Errorf("generate blobs: %w", err)
		}
		_, err = h.Push(ctx, harbor.PushOption{
			Ref:   fmt.Sprintf("%s/%s:%s", projectName, repositoryName, reference),
			Store: store,
			Blobs: derefDescs(blobs),
		})
		if err != nil {
			return nil, fmt.Errorf("push to %s: %w", projectName, err)
		}
	}

	return &pullDiffProjectsData{
		projectNames:   projectNames,
		repositoryName: repositoryName,
		reference:      reference,
	}, nil
}

func (s *PullDifferentProjects) InitWorker(_ context.Context, _ *harbor.Client, _ runner.SharedData) (runner.WorkerState, error) {
	return nil, nil
}

func (s *PullDifferentProjects) Run(ctx context.Context, h *harbor.Client, data runner.SharedData, _ runner.WorkerState) error {
	d := data.(*pullDiffProjectsData)
	projectName := harbor.RandomItem(d.projectNames)
	return h.Pull(ctx, fmt.Sprintf("%s/%s:%s", projectName, d.repositoryName, d.reference))
}

func (s *PullDifferentProjects) Teardown(ctx context.Context, h *harbor.Client, data runner.SharedData) error {
	d := data.(*pullDiffProjectsData)
	for _, projectName := range d.projectNames {
		if err := h.DeleteArtifact(ctx, projectName, d.repositoryName, d.reference); err != nil {
			log.Debugf("cleanup pull-diff %s: %v", projectName, err)
		}
	}
	return nil
}
