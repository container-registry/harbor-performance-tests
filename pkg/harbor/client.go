package harbor

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	rtclient "github.com/go-openapi/runtime/client"
	"github.com/goharbor/perf/pkg/config"
	harborclient "github.com/goharbor/xk6-harbor/pkg/harbor/client"
	"github.com/goharbor/xk6-harbor/pkg/harbor/client/artifact"
	"github.com/goharbor/xk6-harbor/pkg/harbor/client/auditlog"
	"github.com/goharbor/xk6-harbor/pkg/harbor/client/member"
	projectop "github.com/goharbor/xk6-harbor/pkg/harbor/client/project"
	"github.com/goharbor/xk6-harbor/pkg/harbor/client/quota"
	"github.com/goharbor/xk6-harbor/pkg/harbor/client/repository"
	userop "github.com/goharbor/xk6-harbor/pkg/harbor/client/user"
	"github.com/goharbor/xk6-harbor/pkg/harbor/models"
)

var varTrue = true

// Client wraps the Harbor API client, stripping all k6/sobek dependencies.
type Client struct {
	api        *harborclient.HarborAPI
	httpClient *http.Client
	Scheme     string
	Host       string
	Username   string
	Password   string
}

// NewClient creates a Harbor API client from config.
func NewClient(cfg *config.HarborConnection) (*Client, error) {
	rawURL := fmt.Sprintf("%s://%s%s", cfg.Scheme, cfg.Host, harborclient.DefaultBasePath)
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse harbor URL: %w", err)
	}

	apiCfg := harborclient.Config{URL: u}

	if cfg.Username != "" && cfg.Password != "" {
		apiCfg.AuthInfo = rtclient.BasicAuth(cfg.Username, cfg.Password)
	}

	transport := newTransport(cfg.Insecure)
	apiCfg.Transport = transport

	return &Client{
		api:        harborclient.New(apiCfg),
		httpClient: &http.Client{Transport: transport},
		Scheme:     cfg.Scheme,
		Host:       cfg.Host,
		Username:   cfg.Username,
		Password:   cfg.Password,
	}, nil
}

func newTransport(insecure bool) *http.Transport {
	t := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if insecure {
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return t
}

// --- Project operations ---

type ListProjectsResult struct {
	Projects []*models.Project
	Total    int64
}

func (c *Client) ListProjects(ctx context.Context, page, pageSize int64) (*ListProjectsResult, error) {
	params := projectop.NewListProjectsParams()
	params.Page = &page
	params.PageSize = &pageSize

	res, err := c.api.Project.ListProjects(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}

	return &ListProjectsResult{
		Projects: res.Payload,
		Total:    res.XTotalCount,
	}, nil
}

func (c *Client) GetProject(ctx context.Context, projectName string) (*models.Project, error) {
	params := projectop.NewGetProjectParams()
	params.WithProjectNameOrID(projectName)

	res, err := c.api.Project.GetProject(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("get project %s: %w", projectName, err)
	}
	return res.Payload, nil
}

func (c *Client) CreateProject(ctx context.Context, project *models.ProjectReq) (string, error) {
	params := projectop.NewCreateProjectParams()
	params.WithProject(project).WithXResourceNameInLocation(&varTrue)

	res, err := c.api.Project.CreateProject(ctx, params)
	if err != nil {
		return "", fmt.Errorf("create project %s: %w", project.ProjectName, err)
	}
	return nameFromLocation(res.Location), nil
}

func (c *Client) DeleteProject(ctx context.Context, projectName string, force bool) error {
	if force {
		// Delete all repositories first
		pageSize := int64(20)
		page := int64(1)
		for {
			params := repository.NewListRepositoriesParams().WithProjectName(projectName)
			params.Page = &page
			params.PageSize = &pageSize

			resp, err := c.api.Repository.ListRepositories(ctx, params)
			if err != nil {
				return fmt.Errorf("list repos for delete: %w", err)
			}

			for _, repo := range resp.Payload {
				repoName := strings.TrimPrefix(repo.Name, projectName+"/")
				if err := c.DeleteRepository(ctx, projectName, repoName); err != nil {
					return err
				}
			}

			if len(resp.Payload) < int(pageSize) {
				break
			}
		}
	}

	params := projectop.NewDeleteProjectParams()
	params.WithProjectNameOrID(projectName).WithXIsResourceName(&varTrue)

	_, err := c.api.Project.DeleteProject(ctx, params)
	if err != nil {
		return fmt.Errorf("delete project %s: %w", projectName, err)
	}
	return nil
}

type ListProjectLogsResult struct {
	Logs  []*models.AuditLog
	Total int64
}

func (c *Client) ListProjectLogs(ctx context.Context, projectName string) (*ListProjectLogsResult, error) {
	params := projectop.NewGetLogsParams().WithProjectName(projectName)

	res, err := c.api.Project.GetLogs(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list project logs %s: %w", projectName, err)
	}
	return &ListProjectLogsResult{
		Logs:  res.Payload,
		Total: res.XTotalCount,
	}, nil
}

// --- Repository operations ---

type ListRepositoriesResult struct {
	Repositories []*models.Repository
	Total        int64
}

func (c *Client) ListRepositories(ctx context.Context, projectName string) (*ListRepositoriesResult, error) {
	params := repository.NewListRepositoriesParams()
	params.WithProjectName(projectName)

	res, err := c.api.Repository.ListRepositories(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list repositories of %s: %w", projectName, err)
	}
	return &ListRepositoriesResult{
		Repositories: res.Payload,
		Total:        res.XTotalCount,
	}, nil
}

func (c *Client) GetRepository(ctx context.Context, projectName, repositoryName string) (*models.Repository, error) {
	params := repository.NewGetRepositoryParams()
	params.WithProjectName(projectName)
	params.WithRepositoryName(repositoryName)

	res, err := c.api.Repository.GetRepository(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("get repository %s/%s: %w", projectName, repositoryName, err)
	}
	return res.Payload, nil
}

func (c *Client) DeleteRepository(ctx context.Context, projectName, repositoryName string) error {
	params := repository.NewDeleteRepositoryParams()
	params.WithProjectName(projectName).WithRepositoryName(url.PathEscape(repositoryName))

	_, err := c.api.Repository.DeleteRepository(ctx, params)
	if err != nil {
		return fmt.Errorf("delete repository %s/%s: %w", projectName, repositoryName, err)
	}
	return nil
}

// --- Artifact operations ---

type ListArtifactsResult struct {
	Artifacts []*models.Artifact
	Total     int64
}

func (c *Client) ListArtifacts(ctx context.Context, projectName, repositoryName string, opts *ListArtifactsOptions) (*ListArtifactsResult, error) {
	params := artifact.NewListArtifactsParams()
	params.WithProjectName(projectName).WithRepositoryName(url.PathEscape(repositoryName))

	if opts != nil {
		if opts.Page > 0 {
			params.Page = &opts.Page
		}
		if opts.PageSize > 0 {
			params.PageSize = &opts.PageSize
		}
		if opts.WithImmutableStatus {
			params.WithImmutableStatus = &varTrue
		}
		if opts.WithLabel {
			params.WithLabel = &varTrue
		}
		if opts.WithScanOverview {
			params.WithScanOverview = &varTrue
		}
		if opts.WithSignature {
			params.WithSignature = &varTrue
		}
	}

	res, err := c.api.Artifact.ListArtifacts(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list artifacts %s/%s: %w", projectName, repositoryName, err)
	}
	return &ListArtifactsResult{
		Artifacts: res.Payload,
		Total:     res.XTotalCount,
	}, nil
}

type ListArtifactsOptions struct {
	Page                int64
	PageSize            int64
	WithImmutableStatus bool
	WithLabel           bool
	WithScanOverview    bool
	WithSignature       bool
}

func (c *Client) GetArtifact(ctx context.Context, projectName, repositoryName, reference string) (*models.Artifact, error) {
	params := artifact.NewGetArtifactParams()
	params.WithProjectName(projectName)
	params.WithRepositoryName(url.PathEscape(repositoryName))
	params.WithReference(reference)

	res, err := c.api.Artifact.GetArtifact(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("get artifact %s/%s@%s: %w", projectName, repositoryName, reference, err)
	}
	return res.Payload, nil
}

func (c *Client) DeleteArtifact(ctx context.Context, projectName, repositoryName, reference string) error {
	params := artifact.NewDeleteArtifactParams()
	params.WithProjectName(projectName)
	params.WithRepositoryName(url.PathEscape(repositoryName))
	params.WithReference(reference)

	_, err := c.api.Artifact.DeleteArtifact(ctx, params)
	if err != nil {
		return fmt.Errorf("delete artifact %s/%s@%s: %w", projectName, repositoryName, reference, err)
	}
	return nil
}

func (c *Client) ListArtifactTags(ctx context.Context, projectName, repositoryName, reference string, withSignature, withImmutableStatus bool) ([]*models.Tag, error) {
	params := artifact.NewListTagsParams()
	params.WithProjectName(projectName)
	params.WithRepositoryName(url.PathEscape(repositoryName))
	params.WithReference(reference)
	if withSignature {
		params.WithSignature = &varTrue
	}
	if withImmutableStatus {
		params.WithImmutableStatus = &varTrue
	}

	res, err := c.api.Artifact.ListTags(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list artifact tags %s/%s@%s: %w", projectName, repositoryName, reference, err)
	}
	return res.Payload, nil
}

func (c *Client) CreateArtifactTag(ctx context.Context, projectName, repositoryName, reference, newTag string) (string, error) {
	params := artifact.NewCreateTagParams()
	params.WithProjectName(projectName)
	params.WithRepositoryName(url.PathEscape(repositoryName))
	params.WithReference(reference)
	params.WithTag(&models.Tag{Name: newTag})

	res, err := c.api.Artifact.CreateTag(ctx, params)
	if err != nil {
		return "", fmt.Errorf("create tag %s for %s/%s@%s: %w", newTag, projectName, repositoryName, reference, err)
	}
	return res.Location, nil
}

// --- User operations ---

type ListUsersResult struct {
	Users []*models.UserResp
	Total int64
}

func (c *Client) ListUsers(ctx context.Context, page, pageSize int64) (*ListUsersResult, error) {
	params := userop.NewListUsersParams()
	params.Page = &page
	params.PageSize = &pageSize

	res, err := c.api.User.ListUsers(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return &ListUsersResult{
		Users: res.Payload,
		Total: res.XTotalCount,
	}, nil
}

func (c *Client) CreateUser(ctx context.Context, username, password string) (int64, error) {
	params := userop.NewCreateUserParams()
	params.WithUserReq(&models.UserCreationReq{
		Username: username,
		Email:    fmt.Sprintf("%s@goharbor.io", username),
		Password: password,
		Realname: username,
	})

	res, err := c.api.User.CreateUser(ctx, params)
	if err != nil {
		return 0, fmt.Errorf("create user %s: %w", username, err)
	}
	return idFromLocation(res.Location)
}

func (c *Client) DeleteUser(ctx context.Context, userID int64) error {
	params := userop.NewDeleteUserParams()
	params.WithUserID(userID)

	_, err := c.api.User.DeleteUser(ctx, params)
	if err != nil {
		return fmt.Errorf("delete user %d: %w", userID, err)
	}
	return nil
}

type SearchUsersResult struct {
	Users []*models.UserSearchRespItem
	Total int64
}

func (c *Client) SearchUsers(ctx context.Context, username string) (*SearchUsersResult, error) {
	params := userop.NewSearchUsersParams()
	params.Username = username

	res, err := c.api.User.SearchUsers(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("search users %s: %w", username, err)
	}
	return &SearchUsersResult{
		Users: res.Payload,
		Total: res.XTotalCount,
	}, nil
}

// --- Quota operations ---

type ListQuotasResult struct {
	Quotas []*models.Quota
	Total  int64
}

func (c *Client) ListQuotas(ctx context.Context, page, pageSize int64) (*ListQuotasResult, error) {
	params := quota.NewListQuotasParams()
	params.Page = &page
	params.PageSize = &pageSize

	res, err := c.api.Quota.ListQuotas(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list quotas: %w", err)
	}
	return &ListQuotasResult{
		Quotas: res.Payload,
		Total:  res.XTotalCount,
	}, nil
}

// --- Audit log operations ---

type ListAuditLogsResult struct {
	Logs  []*models.AuditLog
	Total int64
}

func (c *Client) ListAuditLogs(ctx context.Context, page, pageSize int64) (*ListAuditLogsResult, error) {
	params := auditlog.NewListAuditLogsParams()
	params.Page = &page
	params.PageSize = &pageSize

	res, err := c.api.Auditlog.ListAuditLogs(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list audit logs: %w", err)
	}
	return &ListAuditLogsResult{
		Logs:  res.Payload,
		Total: res.XTotalCount,
	}, nil
}

// --- Member operations ---

type ListProjectMembersResult struct {
	Members []*models.ProjectMemberEntity
	Total   int64
}

func (c *Client) ListProjectMembers(ctx context.Context, projectName string) (*ListProjectMembersResult, error) {
	params := member.NewListProjectMembersParams()
	params.WithProjectNameOrID(projectName).WithXIsResourceName(&varTrue)

	res, err := c.api.Member.ListProjectMembers(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list project members of %s: %w", projectName, err)
	}
	return &ListProjectMembersResult{
		Members: res.Payload,
		Total:   res.XTotalCount,
	}, nil
}

func (c *Client) CreateProjectMember(ctx context.Context, projectName string, userID, roleID int64) (string, error) {
	params := member.NewCreateProjectMemberParams()
	params.WithProjectNameOrID(projectName).WithXIsResourceName(&varTrue).WithProjectMember(&models.ProjectMember{
		MemberUser: &models.UserEntity{UserID: userID},
		RoleID:     roleID,
	})

	res, err := c.api.Member.CreateProjectMember(ctx, params)
	if err != nil {
		return "", fmt.Errorf("create project member for %s: %w", projectName, err)
	}
	return res.Location, nil
}

// --- V2 / Catalog operations ---

func (c *Client) GetV2(ctx context.Context) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s://%s/v2/", c.Scheme, c.Host), nil)
	if err != nil {
		return 0, err
	}
	req.SetBasicAuth(c.Username, c.Password)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("get /v2/: %w", err)
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

func (c *Client) GetCatalog(ctx context.Context, n int, last string) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s://%s/v2/_catalog", c.Scheme, c.Host), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.Username, c.Password)

	q := req.URL.Query()
	if n != 0 {
		q.Add("n", strconv.Itoa(n))
	}
	if last != "" {
		q.Add("last", last)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get catalog: %w", err)
	}
	defer resp.Body.Close()

	var m map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, fmt.Errorf("decode catalog: %w", err)
	}
	return m, nil
}

// --- Helpers ---

func nameFromLocation(loc string) string {
	parts := strings.Split(loc, "/")
	return parts[len(parts)-1]
}

func idFromLocation(loc string) (int64, error) {
	parts := strings.Split(loc, "/")
	return strconv.ParseInt(parts[len(parts)-1], 10, 64)
}
