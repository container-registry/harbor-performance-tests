package scenarios

import (
	"context"

	"github.com/goharbor/perf/pkg/config"
	"github.com/goharbor/perf/pkg/harbor"
	"github.com/goharbor/perf/pkg/runner"
)

type ListRepositories struct {
	cfg *config.Config
}

type listReposData struct {
	projectNames []string
}

func (s *ListRepositories) Name() string { return "list-repositories" }

func (s *ListRepositories) Setup(_ context.Context, _ *harbor.Client) (runner.SharedData, error) {
	return &listReposData{projectNames: harbor.GetProjectNames(s.cfg)}, nil
}

func (s *ListRepositories) InitWorker(_ context.Context, _ *harbor.Client, _ runner.SharedData) (runner.WorkerState, error) {
	return nil, nil
}

func (s *ListRepositories) Run(ctx context.Context, h *harbor.Client, data runner.SharedData, _ runner.WorkerState) error {
	d := data.(*listReposData)
	_, err := h.ListRepositories(ctx, harbor.RandomItem(d.projectNames))
	return err
}

func (s *ListRepositories) Teardown(_ context.Context, _ *harbor.Client, _ runner.SharedData) error {
	return nil
}
