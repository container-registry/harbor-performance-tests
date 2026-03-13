package prepare

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-openapi/strfmt"
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
	count, err := countManagedProjects(ctx, h, cfg)
	if err != nil {
		return err
	}
	if count < cfg.ProjectsCount {
		return fmt.Errorf("expected >= %d managed projects, got %d", cfg.ProjectsCount, count)
	}
	return nil
}

func (s *ProjectStep) Run(ctx context.Context, h *harbor.Client, cfg *config.Config, workers int) error {
	if cfg.FakeScannerURL != "" {
		name := fmt.Sprintf("fake-scanner-%d", time.Now().UnixMilli())
		urlValue := strfmt.URI(cfg.FakeScannerURL)
		if scannerID, err := h.CreateScanner(ctx, &models.ScannerRegistrationReq{
			Name: &name,
			URL:  &urlValue,
		}); err == nil {
			if err := h.SetScannerAsDefault(ctx, scannerID); err != nil {
				log.Debugf("set fake scanner %s as default: %v", scannerID, err)
			}
		} else {
			log.Debugf("create fake scanner %s: %v", name, err)
		}
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	for i := 0; i < cfg.ProjectsCount; i++ {
		name := harbor.GetProjectName(cfg, i)
		projectName := name
		g.Go(func() error {
			metadata := &models.ProjectMetadata{Public: "false"}
			if cfg.AutoSBOMGeneration {
				autoSBOM := "true"
				metadata.AutoSbomGeneration = &autoSBOM
			}
			_, err := h.CreateProject(ctx, &models.ProjectReq{
				ProjectName: projectName,
				Metadata:    metadata,
			})
			if err != nil && !strings.Contains(err.Error(), "409") {
				return fmt.Errorf("create project %s: %w", projectName, err)
			}
			return nil
		})
	}
	return g.Wait()
}

func (s *ProjectStep) Clean(ctx context.Context, h *harbor.Client, cfg *config.Config, workers int) error {
	projectNames := make([]string, 0, cfg.ProjectsCount)
	page := int64(1)
	pageSize := int64(100)
	for {
		res, err := h.ListProjects(ctx, page, pageSize)
		if err != nil {
			return err
		}
		for _, project := range res.Projects {
			if strings.HasPrefix(project.Name, cfg.ProjectPrefix+"-") {
				projectNames = append(projectNames, project.Name)
			}
		}
		if int64(len(res.Projects)) < pageSize {
			break
		}
		page++
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	for _, projectName := range projectNames {
		projectName := projectName
		g.Go(func() error {
			err := h.DeleteProject(ctx, projectName, true)
			if err != nil {
				log.Debugf("delete project %s: %v", projectName, err)
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
	count, err := countManagedUsers(ctx, h, cfg)
	if err != nil {
		return err
	}
	if count < cfg.UsersCount {
		return fmt.Errorf("expected >= %d managed users, got %d", cfg.UsersCount, count)
	}
	return nil
}

func (s *UserStep) Run(ctx context.Context, h *harbor.Client, cfg *config.Config, workers int) error {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	for i := 0; i < cfg.UsersCount; i++ {
		username := harbor.GetUsername(cfg, i)
		managedUsername := username
		g.Go(func() error {
			_, err := h.CreateUser(ctx, managedUsername, "Harbor12345")
			if err != nil && !strings.Contains(err.Error(), "409") {
				return fmt.Errorf("create user %s: %w", managedUsername, err)
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
			if u.Username != "admin" && strings.HasPrefix(u.Username, cfg.UserPrefix+"-") {
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

func (s *MemberStep) Verify(ctx context.Context, h *harbor.Client, cfg *config.Config) error {
	projectName := harbor.GetProjectName(cfg, 0)
	res, err := h.ListProjectMembers(ctx, projectName)
	if err != nil {
		return err
	}
	if res.Total < int64(cfg.ProjectMembersCountPerProject) {
		return fmt.Errorf("expected >= %d project members in %s, got %d", cfg.ProjectMembersCountPerProject, projectName, res.Total)
	}
	return nil
}

func (s *MemberStep) Run(ctx context.Context, h *harbor.Client, cfg *config.Config, workers int) error {
	// First, get user IDs
	userIDs, err := listManagedUserIDs(ctx, h, cfg)
	if err != nil {
		return err
	}
	if len(userIDs) == 0 {
		return fmt.Errorf("no managed users available for member assignment")
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
			projectName := projectName
			userID := userIDs[(i+j)%len(userIDs)]
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
				projectNameCopy := projectName
				repoNameCopy := repoName
				tagCopy := tag
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
						Ref:   fmt.Sprintf("%s/%s:%s", projectNameCopy, repoNameCopy, tagCopy),
						Store: store,
						Blobs: descs,
					})
					if err != nil {
						log.Debugf("push %s/%s:%s: %v", projectNameCopy, repoNameCopy, tagCopy, err)
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

func (s *TagStep) Verify(ctx context.Context, h *harbor.Client, cfg *config.Config) error {
	projectName := harbor.GetProjectName(cfg, 0)
	repoName := harbor.GetRepositoryName(cfg, 0)
	baseTag := harbor.GetArtifactTag(cfg, 0)
	tags, err := h.ListArtifactTags(ctx, projectName, repoName, baseTag, true, true)
	if err != nil {
		return err
	}
	if len(tags) < cfg.ArtifactTagsCountPerArtifact {
		return fmt.Errorf("expected >= %d tags for %s/%s:%s, got %d", cfg.ArtifactTagsCountPerArtifact, projectName, repoName, baseTag, len(tags))
	}
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
					projectNameCopy := projectName
					repoNameCopy := repoName
					baseTagCopy := baseTag
					newTagCopy := newTag
					g.Go(func() error {
						_, err := h.CreateArtifactTag(ctx, projectNameCopy, repoNameCopy, baseTagCopy, newTagCopy)
						if err != nil {
							log.Debugf("create tag %s: %v", newTagCopy, err)
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

func (s *AuditLogStep) Run(ctx context.Context, h *harbor.Client, cfg *config.Config, workers int) error {
	res, err := h.ListAuditLogs(ctx, 1, 1)
	if err != nil {
		return err
	}

	remaining := cfg.AuditLogsCount - int(res.Total)
	if remaining <= 0 {
		fmt.Printf("    audit log target already satisfied\n")
		return nil
	}

	refs := buildManagedArtifactRefs(cfg)
	if len(refs) == 0 {
		return fmt.Errorf("no artifact references available to generate audit logs")
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	for i := 0; i < remaining; i++ {
		ref := refs[i%len(refs)]
		g.Go(func() error {
			_, err := h.GetManifest(ctx, ref)
			if err != nil {
				return fmt.Errorf("get manifest %s: %w", ref, err)
			}
			return nil
		})
	}

	return g.Wait()
}

func (s *AuditLogStep) Clean(_ context.Context, _ *harbor.Client, _ *config.Config, _ int) error {
	return nil
}

// --- Step 7: Vulnerability Scanning ---

type VulnerabilityStep struct{}

func (s *VulnerabilityStep) Name() string { return "vulnerability-scans" }

func (s *VulnerabilityStep) Verify(ctx context.Context, h *harbor.Client, cfg *config.Config) error {
	if cfg.ScannerURL == "" {
		return nil
	}

	metrics, err := h.GetScanAllMetrics(ctx)
	if err != nil {
		return err
	}
	if metrics.Ongoing || metrics.Total == 0 {
		return fmt.Errorf("scan-all metrics are not ready yet")
	}
	return nil
}

func (s *VulnerabilityStep) Run(ctx context.Context, h *harbor.Client, cfg *config.Config, _ int) error {
	if cfg.ScannerURL == "" {
		fmt.Printf("    no SCANNER_URL configured, skipping vulnerability scanning\n")
		return nil
	}

	name := fmt.Sprintf("scanner-%d", time.Now().UnixMilli())
	urlValue := strfmt.URI(cfg.ScannerURL)
	scannerID, err := h.CreateScanner(ctx, &models.ScannerRegistrationReq{
		Name: &name,
		URL:  &urlValue,
	})
	if err != nil {
		return err
	}
	if err := h.SetScannerAsDefault(ctx, scannerID); err != nil {
		return err
	}
	if err := h.StartScanAll(ctx); err != nil {
		return err
	}

	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		metrics, err := h.GetScanAllMetrics(ctx)
		if err != nil {
			return err
		}
		if !metrics.Ongoing {
			return nil
		}
		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf("scan-all did not finish within 10m")
}

func (s *VulnerabilityStep) Clean(_ context.Context, _ *harbor.Client, _ *config.Config, _ int) error {
	return nil
}

func countManagedProjects(ctx context.Context, h *harbor.Client, cfg *config.Config) (int, error) {
	page := int64(1)
	pageSize := int64(100)
	count := 0

	for {
		res, err := h.ListProjects(ctx, page, pageSize)
		if err != nil {
			return 0, err
		}
		for _, project := range res.Projects {
			if strings.HasPrefix(project.Name, cfg.ProjectPrefix+"-") {
				count++
			}
		}
		if int64(len(res.Projects)) < pageSize {
			break
		}
		page++
	}

	return count, nil
}

func countManagedUsers(ctx context.Context, h *harbor.Client, cfg *config.Config) (int, error) {
	userIDs, err := listManagedUserIDs(ctx, h, cfg)
	if err != nil {
		return 0, err
	}
	return len(userIDs), nil
}

func listManagedUserIDs(ctx context.Context, h *harbor.Client, cfg *config.Config) ([]int64, error) {
	page := int64(1)
	pageSize := int64(100)
	userIDs := make([]int64, 0, cfg.UsersCount)

	for {
		res, err := h.ListUsers(ctx, page, pageSize)
		if err != nil {
			return nil, err
		}
		for _, u := range res.Users {
			if strings.HasPrefix(u.Username, cfg.UserPrefix+"-") {
				userIDs = append(userIDs, u.UserID)
			}
		}
		if int64(len(res.Users)) < pageSize {
			break
		}
		page++
	}

	return userIDs, nil
}

func buildManagedArtifactRefs(cfg *config.Config) []string {
	refs := make([]string, 0, cfg.ProjectsCount*cfg.RepositoriesCountPerProject*cfg.ArtifactsCountPerRepository)
	for i := 0; i < cfg.ProjectsCount; i++ {
		projectName := harbor.GetProjectName(cfg, i)
		for j := 0; j < cfg.RepositoriesCountPerProject; j++ {
			repoName := harbor.GetRepositoryName(cfg, j)
			for k := 0; k < cfg.ArtifactsCountPerRepository; k++ {
				tag := harbor.GetArtifactTag(cfg, k)
				refs = append(refs, fmt.Sprintf("%s/%s:%s", projectName, repoName, tag))
			}
		}
	}
	return refs
}
