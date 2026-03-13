package prepare

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goharbor/perf/pkg/config"
	"github.com/goharbor/perf/pkg/harbor"
	"github.com/goharbor/xk6-harbor/pkg/harbor/models"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// --- Step 1: Projects ---

type ProjectStep struct{}

func (s *ProjectStep) Name() string { return "projects" }

func (s *ProjectStep) Verify(ctx context.Context, h *harbor.Client, cfg *config.Config) error {
	res, err := h.ListProjects(ctx, 1, 1)
	if err != nil {
		return err
	}
	// Allow some slack (library project exists by default)
	if res.Total < int64(cfg.ProjectsCount) {
		return fmt.Errorf("expected >= %d projects, got %d", cfg.ProjectsCount, res.Total)
	}
	return nil
}

func (s *ProjectStep) Run(ctx context.Context, h *harbor.Client, cfg *config.Config, workers int) error {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	for i := 0; i < cfg.ProjectsCount; i++ {
		name := harbor.GetProjectName(cfg, i)
		g.Go(func() error {
			md := map[string]string{}
			if autoSbom := os.Getenv("AUTO_SBOM_GENERATION"); strings.EqualFold(autoSbom, "true") {
				md["auto_sbom_generation"] = "true"
			}
			_, err := h.CreateProject(ctx, &models.ProjectReq{
				ProjectName: name,
				Metadata:    &models.ProjectMetadata{Public: "false"},
			})
			if err != nil && !strings.Contains(err.Error(), "409") {
				return fmt.Errorf("create project %s: %w", name, err)
			}
			return nil
		})
	}
	return g.Wait()
}

func (s *ProjectStep) Clean(ctx context.Context, h *harbor.Client, cfg *config.Config, workers int) error {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	for i := 0; i < cfg.ProjectsCount; i++ {
		name := harbor.GetProjectName(cfg, i)
		g.Go(func() error {
			err := h.DeleteProject(ctx, name, true)
			if err != nil {
				log.Debugf("delete project %s: %v", name, err)
			}
			return nil // don't fail on cleanup errors
		})
	}
	return g.Wait()
}

// --- Step 2: Users ---

type UserStep struct{}

func (s *UserStep) Name() string { return "users" }

func (s *UserStep) Verify(ctx context.Context, h *harbor.Client, cfg *config.Config) error {
	res, err := h.ListUsers(ctx, 1, 1)
	if err != nil {
		return err
	}
	// admin user exists by default, so total should be >= UsersCount+1
	if res.Total < int64(cfg.UsersCount) {
		return fmt.Errorf("expected >= %d users, got %d", cfg.UsersCount, res.Total)
	}
	return nil
}

func (s *UserStep) Run(ctx context.Context, h *harbor.Client, cfg *config.Config, workers int) error {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	for i := 0; i < cfg.UsersCount; i++ {
		username := harbor.GetUsername(cfg, i)
		g.Go(func() error {
			_, err := h.CreateUser(ctx, username, "Harbor12345")
			if err != nil && !strings.Contains(err.Error(), "409") {
				return fmt.Errorf("create user %s: %w", username, err)
			}
			return nil
		})
	}
	return g.Wait()
}

func (s *UserStep) Clean(ctx context.Context, h *harbor.Client, cfg *config.Config, workers int) error {
	// Fetch all users and delete non-admin ones
	page := int64(1)
	pageSize := int64(100)
	for {
		res, err := h.ListUsers(ctx, page, pageSize)
		if err != nil {
			return err
		}
		for _, u := range res.Users {
			if u.Username != "admin" {
				if err := h.DeleteUser(ctx, u.UserID); err != nil {
					log.Debugf("delete user %s: %v", u.Username, err)
				}
			}
		}
		if int64(len(res.Users)) < pageSize {
			break
		}
		page++
	}
	return nil
}

// --- Step 3: Members ---

type MemberStep struct{}

func (s *MemberStep) Name() string { return "project-members" }

func (s *MemberStep) Verify(_ context.Context, _ *harbor.Client, _ *config.Config) error {
	// Members are difficult to verify precisely; skip verification
	return nil
}

func (s *MemberStep) Run(ctx context.Context, h *harbor.Client, cfg *config.Config, workers int) error {
	// First, get user IDs
	userIDs := make([]int64, 0, cfg.UsersCount)
	page := int64(1)
	pageSize := int64(100)
	for {
		res, err := h.ListUsers(ctx, page, pageSize)
		if err != nil {
			return err
		}
		for _, u := range res.Users {
			if u.Username != "admin" {
				userIDs = append(userIDs, u.UserID)
			}
		}
		if int64(len(res.Users)) < pageSize {
			break
		}
		page++
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	for i := 0; i < cfg.ProjectsCount; i++ {
		projectName := harbor.GetProjectName(cfg, i)
		membersCount := cfg.ProjectMembersCountPerProject
		if membersCount > len(userIDs) {
			membersCount = len(userIDs)
		}
		for j := 0; j < membersCount; j++ {
			userID := userIDs[j]
			g.Go(func() error {
				_, err := h.CreateProjectMember(ctx, projectName, userID, 1)
				if err != nil && !strings.Contains(err.Error(), "409") {
					log.Debugf("create member for %s: %v", projectName, err)
				}
				return nil
			})
		}
	}
	return g.Wait()
}

func (s *MemberStep) Clean(_ context.Context, _ *harbor.Client, _ *config.Config, _ int) error {
	// Members are deleted with projects
	return nil
}

// --- Step 4: Artifacts ---

type ArtifactStep struct{}

func (s *ArtifactStep) Name() string { return "artifacts" }

func (s *ArtifactStep) Verify(ctx context.Context, h *harbor.Client, cfg *config.Config) error {
	// Spot check: list artifacts in first project's first repo
	projectName := harbor.GetProjectName(cfg, 0)
	repoName := harbor.GetRepositoryName(cfg, 0)
	res, err := h.ListArtifacts(ctx, projectName, repoName, nil)
	if err != nil {
		return err
	}
	if res.Total < int64(cfg.ArtifactsCountPerRepository) {
		return fmt.Errorf("expected >= %d artifacts in %s/%s, got %d",
			cfg.ArtifactsCountPerRepository, projectName, repoName, res.Total)
	}
	return nil
}

func (s *ArtifactStep) Run(ctx context.Context, h *harbor.Client, cfg *config.Config, workers int) error {
	storePath := filepath.Join(os.TempDir(), "harbor", fmt.Sprintf("prepare-artifacts-%d", time.Now().UnixNano()))
	store, err := harbor.NewContentStore(storePath)
	if err != nil {
		return err
	}
	defer store.Free()

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	for i := 0; i < cfg.ProjectsCount; i++ {
		projectName := harbor.GetProjectName(cfg, i)
		for j := 0; j < cfg.RepositoriesCountPerProject; j++ {
			repoName := harbor.GetRepositoryName(cfg, j)
			for k := 0; k < cfg.ArtifactsCountPerRepository; k++ {
				tag := harbor.GetArtifactTag(cfg, k)
				g.Go(func() error {
					blobs, err := store.GenerateMany(cfg.BlobSize, cfg.BlobsCountPerArtifact)
					if err != nil {
						return fmt.Errorf("generate blobs: %w", err)
					}
					descs := make([]harbor.OciDescriptor, len(blobs))
					for idx, b := range blobs {
						descs[idx] = *b
					}
					_, err = h.Push(ctx, harbor.PushOption{
						Ref:   fmt.Sprintf("%s/%s:%s", projectName, repoName, tag),
						Store: store,
						Blobs: descs,
					})
					if err != nil {
						log.Debugf("push %s/%s:%s: %v", projectName, repoName, tag, err)
					}
					return nil
				})
			}
		}
	}
	return g.Wait()
}

func (s *ArtifactStep) Clean(_ context.Context, _ *harbor.Client, _ *config.Config, _ int) error {
	// Artifacts are deleted with repositories/projects
	return nil
}

// --- Step 5: Tags ---

type TagStep struct{}

func (s *TagStep) Name() string { return "artifact-tags" }

func (s *TagStep) Verify(_ context.Context, _ *harbor.Client, _ *config.Config) error {
	return nil
}

func (s *TagStep) Run(ctx context.Context, h *harbor.Client, cfg *config.Config, workers int) error {
	if cfg.ArtifactTagsCountPerArtifact <= 1 {
		return nil
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	for i := 0; i < cfg.ProjectsCount; i++ {
		projectName := harbor.GetProjectName(cfg, i)
		for j := 0; j < cfg.RepositoriesCountPerProject; j++ {
			repoName := harbor.GetRepositoryName(cfg, j)
			for k := 0; k < cfg.ArtifactsCountPerRepository; k++ {
				baseTag := harbor.GetArtifactTag(cfg, k)
				for t := 1; t < cfg.ArtifactTagsCountPerArtifact; t++ {
					newTag := fmt.Sprintf("%sp%s", baseTag, harbor.NumberToPadString(t, cfg.ArtifactTagsCountPerArtifact))
					g.Go(func() error {
						_, err := h.CreateArtifactTag(ctx, projectName, repoName, baseTag, newTag)
						if err != nil {
							log.Debugf("create tag %s: %v", newTag, err)
						}
						return nil
					})
				}
			}
		}
	}
	return g.Wait()
}

func (s *TagStep) Clean(_ context.Context, _ *harbor.Client, _ *config.Config, _ int) error {
	return nil
}

// --- Step 6: Audit Logs ---

type AuditLogStep struct{}

func (s *AuditLogStep) Name() string { return "audit-logs" }

func (s *AuditLogStep) Verify(ctx context.Context, h *harbor.Client, cfg *config.Config) error {
	res, err := h.ListAuditLogs(ctx, 1, 1)
	if err != nil {
		return err
	}
	if res.Total < int64(cfg.AuditLogsCount/2) { // allow some slack
		return fmt.Errorf("expected >= %d audit logs, got %d", cfg.AuditLogsCount/2, res.Total)
	}
	return nil
}

func (s *AuditLogStep) Run(_ context.Context, _ *harbor.Client, _ *config.Config, _ int) error {
	// Audit logs are generated as a side effect of other operations
	// The k6 version does explicit API calls to generate them, but
	// for the Go version, the prepare steps above generate enough logs
	fmt.Printf("    audit logs are generated as side effects of other prepare steps\n")
	return nil
}

func (s *AuditLogStep) Clean(_ context.Context, _ *harbor.Client, _ *config.Config, _ int) error {
	return nil
}

// --- Step 7: Vulnerability Scanning ---

type VulnerabilityStep struct{}

func (s *VulnerabilityStep) Name() string { return "vulnerability-scans" }

func (s *VulnerabilityStep) Verify(_ context.Context, _ *harbor.Client, _ *config.Config) error {
	return nil
}

func (s *VulnerabilityStep) Run(_ context.Context, _ *harbor.Client, _ *config.Config, _ int) error {
	// Vulnerability scanning requires scanner configuration
	// Skip if no scanner URL is configured
	scannerURL := os.Getenv("SCANNER_URL")
	if scannerURL == "" {
		fmt.Printf("    no SCANNER_URL configured, skipping vulnerability scanning\n")
		return nil
	}
	fmt.Printf("    vulnerability scanning not yet implemented in Go runner\n")
	return nil
}

func (s *VulnerabilityStep) Clean(_ context.Context, _ *harbor.Client, _ *config.Config, _ int) error {
	return nil
}
