package scenarios

import (
	"context"

	"github.com/goharbor/perf/pkg/config"
	"github.com/goharbor/perf/pkg/harbor"
	"github.com/goharbor/perf/pkg/runner"
)

type SearchUsersScenario struct {
	cfg *config.Config
}

type searchUsersData struct {
	usernames []string
}

func (s *SearchUsersScenario) Name() string { return "search-users" }

func (s *SearchUsersScenario) Setup(_ context.Context, _ *harbor.Client) (runner.SharedData, error) {
	return &searchUsersData{usernames: harbor.GetUsernames(s.cfg)}, nil
}

func (s *SearchUsersScenario) InitWorker(_ context.Context, _ *harbor.Client, _ runner.SharedData) (runner.WorkerState, error) {
	return nil, nil
}

func (s *SearchUsersScenario) Run(ctx context.Context, h *harbor.Client, data runner.SharedData, _ runner.WorkerState) error {
	d := data.(*searchUsersData)
	_, err := h.SearchUsers(ctx, harbor.RandomItem(d.usernames))
	return err
}

func (s *SearchUsersScenario) Teardown(_ context.Context, _ *harbor.Client, _ runner.SharedData) error {
	return nil
}
