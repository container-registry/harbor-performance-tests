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

type PullSameProject struct {
	cfg *config.Config
}

type pullSameProjectData struct {
	projectName    string
	repositoryName string
	reference      string
}

func (s *PullSameProject) Name() string { return "pull-artifacts-from-same-project" }

func (s *PullSameProject) Setup(ctx context.Context, h *harbor.Client) (runner.SharedData, error) {
	projectName := harbor.RandomItem(harbor.GetProjectNames(s.cfg))
	repositoryName := fmt.Sprintf("repository-%d", time.Now().UnixMilli())
	reference := fmt.Sprintf("tag-%d", time.Now().UnixMilli())

	storePath := filepath.Join(os.TempDir(), "harbor", fmt.Sprintf("pull-same-%d", time.Now().UnixNano()))
	store, err := harbor.NewContentStore(storePath)
	if err != nil {
		return nil, fmt.Errorf("create content store: %w", err)
	}
	defer store.Free()

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
		return nil, fmt.Errorf("push setup artifact: %w", err)
	}

	return &pullSameProjectData{
		projectName:    projectName,
		repositoryName: repositoryName,
		reference:      reference,
	}, nil
}

func (s *PullSameProject) InitWorker(_ context.Context, _ *harbor.Client, _ runner.SharedData) (runner.WorkerState, error) {
	return nil, nil
}

func (s *PullSameProject) Run(ctx context.Context, h *harbor.Client, data runner.SharedData, _ runner.WorkerState) error {
	d := data.(*pullSameProjectData)
	return h.Pull(ctx, fmt.Sprintf("%s/%s:%s", d.projectName, d.repositoryName, d.reference))
}

func (s *PullSameProject) Teardown(ctx context.Context, h *harbor.Client, data runner.SharedData) error {
	d := data.(*pullSameProjectData)
	if err := h.DeleteArtifact(ctx, d.projectName, d.repositoryName, d.reference); err != nil {
		log.Debugf("cleanup pull-same: %v", err)
	}
	return nil
}
