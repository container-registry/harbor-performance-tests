package scenarios

import (
	"context"

	"github.com/goharbor/perf/pkg/config"
	"github.com/goharbor/perf/pkg/harbor"
	"github.com/goharbor/perf/pkg/runner"
)

type GetArtifactByTag struct {
	cfg *config.Config
}

type getArtifactByTagData struct {
	projectName    string
	repositoryName string
	reference      string
}

func (s *GetArtifactByTag) Name() string { return "get-artifact-by-tag" }

func (s *GetArtifactByTag) Setup(ctx context.Context, h *harbor.Client) (runner.SharedData, error) {
	res, err := h.ListProjects(ctx, 1, 10)
	if err != nil {
		return nil, err
	}
	for _, p := range res.Projects {
		repos, err := h.ListRepositories(ctx, p.Name)
		if err != nil {
			continue
		}
		for _, repo := range repos.Repositories {
			repoName := harbor.StripProjectPrefix(p.Name, repo.Name)
			arts, err := h.ListArtifacts(ctx, p.Name, repoName, nil)
			if err != nil {
				continue
			}
			for _, a := range arts.Artifacts {
				if a.Tags != nil && len(a.Tags) > 0 {
					return &getArtifactByTagData{
						projectName:    p.Name,
						repositoryName: repoName,
						reference:      harbor.RandomItem(a.Tags).Name,
					}, nil
				}
			}
		}
	}
	return &getArtifactByTagData{}, nil
}

func (s *GetArtifactByTag) InitWorker(_ context.Context, _ *harbor.Client, _ runner.SharedData) (runner.WorkerState, error) {
	return nil, nil
}

func (s *GetArtifactByTag) Run(ctx context.Context, h *harbor.Client, data runner.SharedData, _ runner.WorkerState) error {
	d := data.(*getArtifactByTagData)
	_, err := h.GetArtifact(ctx, d.projectName, d.repositoryName, d.reference)
	return err
}

func (s *GetArtifactByTag) Teardown(_ context.Context, _ *harbor.Client, _ runner.SharedData) error {
	return nil
}
