package scenarios

import (
	"context"

	"github.com/goharbor/perf/pkg/config"
	"github.com/goharbor/perf/pkg/harbor"
	"github.com/goharbor/perf/pkg/runner"
)

type ListProjectMembers struct {
	cfg *config.Config
}

type listProjectMembersData struct {
	projectNames []string
}

func (s *ListProjectMembers) Name() string { return "list-project-members" }

func (s *ListProjectMembers) Setup(_ context.Context, _ *harbor.Client) (runner.SharedData, error) {
	return &listProjectMembersData{projectNames: harbor.GetProjectNames(s.cfg)}, nil
}

func (s *ListProjectMembers) InitWorker(_ context.Context, _ *harbor.Client, _ runner.SharedData) (runner.WorkerState, error) {
	return nil, nil
}

func (s *ListProjectMembers) Run(ctx context.Context, h *harbor.Client, data runner.SharedData, _ runner.WorkerState) error {
	d := data.(*listProjectMembersData)
	_, err := h.ListProjectMembers(ctx, harbor.RandomItem(d.projectNames))
	return err
}

func (s *ListProjectMembers) Teardown(_ context.Context, _ *harbor.Client, _ runner.SharedData) error {
	return nil
}
