package scenarios

import (
	"context"

	"github.com/goharbor/perf/pkg/config"
	"github.com/goharbor/perf/pkg/harbor"
	"github.com/goharbor/perf/pkg/runner"
)

type ListArtifacts struct {
	cfg *config.Config
}

type repoRef struct {
	projectName    string
	repositoryName string
}

type listArtifactsData struct {
	repos []repoRef
}

func (s *ListArtifacts) Name() string { return "list-artifacts" }

func (s *ListArtifacts) Setup(_ context.Context, _ *harbor.Client) (runner.SharedData, error) {
	var repos []repoRef
	for i := 0; i < s.cfg.ProjectsCount; i++ {
		for j := 0; j < s.cfg.RepositoriesCountPerProject; j++ {
			repos = append(repos, repoRef{
				projectName:    harbor.GetProjectName(s.cfg, i),
				repositoryName: harbor.GetRepositoryName(s.cfg, j),
			})
		}
	}
	return &listArtifactsData{repos: repos}, nil
}

func (s *ListArtifacts) InitWorker(_ context.Context, _ *harbor.Client, _ runner.SharedData) (runner.WorkerState, error) {
	return nil, nil
}

func (s *ListArtifacts) Run(ctx context.Context, h *harbor.Client, data runner.SharedData, _ runner.WorkerState) error {
	d := data.(*listArtifactsData)
	r := harbor.RandomItem(d.repos)
	_, err := h.ListArtifacts(ctx, r.projectName, r.repositoryName, &harbor.ListArtifactsOptions{
		WithImmutableStatus: true,
		WithLabel:           true,
		WithScanOverview:    true,
		WithSignature:       true,
	})
	return err
}

func (s *ListArtifacts) Teardown(_ context.Context, _ *harbor.Client, _ runner.SharedData) error {
	return nil
}
