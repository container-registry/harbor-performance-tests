package scenarios

import (
	"github.com/goharbor/perf/pkg/config"
	"github.com/goharbor/perf/pkg/runner"
)

// All returns all registered scenarios.
func All(cfg *config.Config) []runner.Scenario {
	return []runner.Scenario{
		// API read tests
		&ListProjects{cfg: cfg},
		&ListRepositories{cfg: cfg},
		&ListArtifacts{cfg: cfg},
		&ListArtifactTags{cfg: cfg},
		&ListUsers{cfg: cfg},
		&ListQuotas{cfg: cfg},
		&ListAuditLogs{cfg: cfg},
		&ListProjectLogs{cfg: cfg},
		&ListProjectMembers{cfg: cfg},
		&GetProject{cfg: cfg},
		&GetRepository{cfg: cfg},
		&GetArtifactByTag{cfg: cfg},
		&GetArtifactByDigest{cfg: cfg},
		&GetCatalog{cfg: cfg},
		&GetV2{cfg: cfg},
		&SearchUsersScenario{cfg: cfg},
		// OCI distribution tests
		&PushSameProject{cfg: cfg},
		&PushDifferentProjects{cfg: cfg},
		&PullSameProject{cfg: cfg},
		&PullDifferentProjects{cfg: cfg},
	}
}

// APIOnly returns only API read scenarios (no push/pull).
func APIOnly(cfg *config.Config) []runner.Scenario {
	all := All(cfg)
	var result []runner.Scenario
	for _, s := range all {
		name := s.Name()
		if name != "push-artifacts-to-same-project" &&
			name != "push-artifacts-to-different-projects" &&
			name != "pull-artifacts-from-same-project" &&
			name != "pull-artifacts-from-different-projects" {
			result = append(result, s)
		}
	}
	return result
}

// ByName returns a scenario by name.
func ByName(cfg *config.Config, name string) runner.Scenario {
	for _, s := range All(cfg) {
		if s.Name() == name {
			return s
		}
	}
	return nil
}

// Names returns all scenario names.
func Names(cfg *config.Config) []string {
	all := All(cfg)
	names := make([]string, len(all))
	for i, s := range all {
		names[i] = s.Name()
	}
	return names
}
