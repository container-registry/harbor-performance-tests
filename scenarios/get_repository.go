package scenarios

import (
	"context"

	"github.com/goharbor/perf/pkg/config"
	"github.com/goharbor/perf/pkg/harbor"
	"github.com/goharbor/perf/pkg/runner"
)

type GetRepository struct {
	cfg *config.Config
}

type getRepoData struct {
	projectName    string
	repositoryName string
}

func (s *GetRepository) Name() string { return "get-repository" }

func (s *GetRepository) Setup(ctx context.Context, h *harbor.Client) (runner.SharedData, error) {
	res, err := h.ListProjects(ctx, 1, 10)
	if err != nil {
		return nil, err
	}
	for _, p := range res.Projects {
		repos, err := h.ListRepositories(ctx, p.Name)
		if err != nil {
			continue
		}
		if len(repos.Repositories) > 0 {
			repo := harbor.RandomItem(repos.Repositories)
			return &getRepoData{
				projectName:    p.Name,
				repositoryName: harbor.StripProjectPrefix(p.Name, repo.Name),
			}, nil
		}
	}
	return &getRepoData{}, nil
}

func (s *GetRepository) InitWorker(_ context.Context, _ *harbor.Client, _ runner.SharedData) (runner.WorkerState, error) {
	return nil, nil
}

func (s *GetRepository) Run(ctx context.Context, h *harbor.Client, data runner.SharedData, _ runner.WorkerState) error {
	d := data.(*getRepoData)
	_, err := h.GetRepository(ctx, d.projectName, d.repositoryName)
	return err
}

func (s *GetRepository) Teardown(_ context.Context, _ *harbor.Client, _ runner.SharedData) error {
	return nil
}
