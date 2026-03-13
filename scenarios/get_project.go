package scenarios

import (
	"context"

	"github.com/goharbor/perf/pkg/config"
	"github.com/goharbor/perf/pkg/harbor"
	"github.com/goharbor/perf/pkg/runner"
)

type GetProject struct {
	cfg *config.Config
}

type getProjectData struct {
	projectName string
}

func (s *GetProject) Name() string { return "get-project" }

func (s *GetProject) Setup(ctx context.Context, h *harbor.Client) (runner.SharedData, error) {
	res, err := h.ListProjects(ctx, 1, 10)
	if err != nil {
		return nil, err
	}
	if len(res.Projects) == 0 {
		return &getProjectData{}, nil
	}
	return &getProjectData{projectName: harbor.RandomItem(res.Projects).Name}, nil
}

func (s *GetProject) InitWorker(_ context.Context, _ *harbor.Client, _ runner.SharedData) (runner.WorkerState, error) {
	return nil, nil
}

func (s *GetProject) Run(ctx context.Context, h *harbor.Client, data runner.SharedData, _ runner.WorkerState) error {
	d := data.(*getProjectData)
	_, err := h.GetProject(ctx, d.projectName)
	return err
}

func (s *GetProject) Teardown(_ context.Context, _ *harbor.Client, _ runner.SharedData) error {
	return nil
}
