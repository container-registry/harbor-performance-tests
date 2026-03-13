package scenarios

import (
	"context"

	"github.com/goharbor/perf/pkg/config"
	"github.com/goharbor/perf/pkg/harbor"
	"github.com/goharbor/perf/pkg/runner"
)

type GetArtifactByDigest struct {
	cfg *config.Config
}

type getArtifactByDigestData struct {
	projectName    string
	repositoryName string
	reference      string
}

func (s *GetArtifactByDigest) Name() string { return "get-artifact-by-digest" }

func (s *GetArtifactByDigest) Setup(ctx context.Context, h *harbor.Client) (runner.SharedData, error) {
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
			if len(arts.Artifacts) > 0 {
				a := harbor.RandomItem(arts.Artifacts)
				return &getArtifactByDigestData{
					projectName:    p.Name,
					repositoryName: repoName,
					reference:      a.Digest,
				}, nil
			}
		}
	}
	return &getArtifactByDigestData{}, nil
}

func (s *GetArtifactByDigest) InitWorker(_ context.Context, _ *harbor.Client, _ runner.SharedData) (runner.WorkerState, error) {
	return nil, nil
}

func (s *GetArtifactByDigest) Run(ctx context.Context, h *harbor.Client, data runner.SharedData, _ runner.WorkerState) error {
	d := data.(*getArtifactByDigestData)
	_, err := h.GetArtifact(ctx, d.projectName, d.repositoryName, d.reference)
	return err
}

func (s *GetArtifactByDigest) Teardown(_ context.Context, _ *harbor.Client, _ runner.SharedData) error {
	return nil
}
