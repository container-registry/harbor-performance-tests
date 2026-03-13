package scenarios

import (
	"context"

	"github.com/goharbor/perf/pkg/config"
	"github.com/goharbor/perf/pkg/harbor"
	"github.com/goharbor/perf/pkg/runner"
)

type GetCatalog struct {
	cfg *config.Config
}

type getCatalogData struct {
	repositories []string
}

func (s *GetCatalog) Name() string { return "get-catalog" }

func (s *GetCatalog) Setup(_ context.Context, _ *harbor.Client) (runner.SharedData, error) {
	var repos []string
	for i := 0; i < s.cfg.ProjectsCount; i++ {
		for j := 0; j < s.cfg.RepositoriesCountPerProject; j++ {
			repos = append(repos, harbor.GetProjectName(s.cfg, i)+"/"+harbor.GetRepositoryName(s.cfg, j))
		}
	}
	return &getCatalogData{repositories: repos}, nil
}

func (s *GetCatalog) InitWorker(_ context.Context, _ *harbor.Client, _ runner.SharedData) (runner.WorkerState, error) {
	return nil, nil
}

func (s *GetCatalog) Run(ctx context.Context, h *harbor.Client, data runner.SharedData, _ runner.WorkerState) error {
	d := data.(*getCatalogData)
	n := harbor.RandomIntBetween(1, len(d.repositories))
	last := harbor.RandomItem(d.repositories)
	_, err := h.GetCatalog(ctx, n, last)
	return err
}

func (s *GetCatalog) Teardown(_ context.Context, _ *harbor.Client, _ runner.SharedData) error {
	return nil
}
