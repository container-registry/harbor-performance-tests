package harbor

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/goharbor/perf/pkg/config"
)

// RandomItem returns a random element from a slice.
func RandomItem[T any](items []T) T {
	return items[rand.IntN(len(items))]
}

// RandomIntBetween returns a random int in [min, max] inclusive.
func RandomIntBetween(min, max int) int {
	return min + rand.IntN(max-min+1)
}

// NumberToPadString pads num with leading zeros based on maxNum's digit count.
func NumberToPadString(num, maxNum int) string {
	width := len(fmt.Sprintf("%d", maxNum))
	return fmt.Sprintf("%0*d", width, num)
}

// GetProjectName returns the project name for index i (0-based).
func GetProjectName(cfg *config.Config, i int) string {
	return fmt.Sprintf("%s-%s", cfg.ProjectPrefix, NumberToPadString(i+1, cfg.ProjectsCount))
}

// GetProjectNames returns all project names for the configured size.
func GetProjectNames(cfg *config.Config) []string {
	names := make([]string, cfg.ProjectsCount)
	for i := range names {
		names[i] = GetProjectName(cfg, i)
	}
	return names
}

// GetUsername returns the username for index i (0-based).
func GetUsername(cfg *config.Config, i int) string {
	return fmt.Sprintf("%s-%s", cfg.UserPrefix, NumberToPadString(i+1, cfg.UsersCount))
}

// GetUsernames returns all usernames for the configured size.
func GetUsernames(cfg *config.Config) []string {
	names := make([]string, cfg.UsersCount)
	for i := range names {
		names[i] = GetUsername(cfg, i)
	}
	return names
}

// GetRepositoryName returns the repository name for index i (0-based).
func GetRepositoryName(cfg *config.Config, i int) string {
	return fmt.Sprintf("repository-%s", NumberToPadString(i+1, cfg.RepositoriesCountPerProject))
}

// GetArtifactTag returns the artifact tag for index i (0-based).
func GetArtifactTag(cfg *config.Config, i int) string {
	return fmt.Sprintf("v%s", NumberToPadString(i+1, cfg.ArtifactsCountPerRepository))
}

// StripProjectPrefix removes the "project/" prefix from a repository name.
func StripProjectPrefix(projectName, fullRepoName string) string {
	return strings.TrimPrefix(fullRepoName, projectName+"/")
}
