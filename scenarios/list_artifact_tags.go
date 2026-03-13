package scenarios

import (
	"context"

	"github.com/goharbor/perf/pkg/config"
	"github.com/goharbor/perf/pkg/harbor"
	"github.com/goharbor/perf/pkg/runner"
)

type ListArtifactTags struct {
	cfg *config.Config
}

type artifactRef struct {
	projectName    string
	repositoryName string
	reference      string
}

type listArtifactTagsData struct {
	artifacts []artifactRef
}

func (s *ListArtifactTags) Name() string { return "list-artifact-tags" }

func (s *ListArtifactTags) Setup(_ context.Context, _ *harbor.Client) (runner.SharedData, error) {
	var artifacts []artifactRef
	for i := 0; i < s.cfg.ProjectsCount; i++ {
		for j := 0; j < s.cfg.RepositoriesCountPerProject; j++ {
			for k := 0; k < s.cfg.ArtifactsCountPerRepository; k++ {
				artifacts = append(artifacts, artifactRef{
					projectName:    harbor.GetProjectName(s.cfg, i),
					repositoryName: harbor.GetRepositoryName(s.cfg, j),
					reference:      harbor.GetArtifactTag(s.cfg, k),
				})
			}
		}
	}
	return &listArtifactTagsData{artifacts: artifacts}, nil
}

func (s *ListArtifactTags) InitWorker(_ context.Context, _ *harbor.Client, _ runner.SharedData) (runner.WorkerState, error) {
	return nil, nil
}

func (s *ListArtifactTags) Run(ctx context.Context, h *harbor.Client, data runner.SharedData, _ runner.WorkerState) error {
	d := data.(*listArtifactTagsData)
	a := harbor.RandomItem(d.artifacts)
	_, err := h.ListArtifactTags(ctx, a.projectName, a.repositoryName, a.reference, true, true)
	return err
}

func (s *ListArtifactTags) Teardown(_ context.Context, _ *harbor.Client, _ runner.SharedData) error {
	return nil
}
