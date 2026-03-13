package config

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds all configuration for a performance test run.
type Config struct {
	Harbor     HarborConnection
	Size       SizePreset
	VUs        int
	Iterations int

	// Size-derived values
	ProjectsCount                 int
	RepositoriesCountPerProject   int
	ArtifactsCountPerRepository   int
	ArtifactTagsCountPerArtifact  int
	UsersCount                    int
	ProjectMembersCountPerProject int
	AuditLogsCount                int
	BlobSize                      string
	BlobsCountPerArtifact         int

	// Naming
	ProjectPrefix      string
	UserPrefix         string
	ScannerURL         string
	FakeScannerURL     string
	AutoSBOMGeneration bool

	// Output
	OutputDir     string
	CSVOutput     bool
	JSONOutput    bool
	ReportEnabled bool

	// Dataset
	DatasetPolicy DatasetPolicy
}

// HarborConnection holds Harbor connection details.
type HarborConnection struct {
	Scheme   string
	Host     string
	Username string
	Password string
	Insecure bool
}

// SizePreset is the dataset size preset.
type SizePreset string

const (
	SizeCI     SizePreset = "ci"
	SizeSmall  SizePreset = "small"
	SizeMedium SizePreset = "medium"
)

// DatasetPolicy controls how data preparation behaves.
type DatasetPolicy string

const (
	PolicyFresh  DatasetPolicy = "fresh"
	PolicyVerify DatasetPolicy = "verify"
	PolicyReuse  DatasetPolicy = "reuse"
)

// DatasetFingerprint uniquely identifies a dataset configuration.
type DatasetFingerprint struct {
	SizePreset         string `json:"sizePreset"`
	ProjectPrefix      string `json:"projectPrefix"`
	UserPrefix         string `json:"userPrefix"`
	Projects           int    `json:"projects"`
	ReposPerProject    int    `json:"reposPerProject"`
	ArtifactsPerRepo   int    `json:"artifactsPerRepo"`
	TagsPerArtifact    int    `json:"tagsPerArtifact"`
	Users              int    `json:"users"`
	MembersPerProject  int    `json:"membersPerProject"`
	AuditLogs          int    `json:"auditLogs"`
	BlobSize           string `json:"blobSize"`
	BlobsPerArtifact   int    `json:"blobsPerArtifact"`
	ScannerURL         string `json:"scannerURL,omitempty"`
	FakeScannerURL     string `json:"fakeScannerURL,omitempty"`
	AutoSBOMGeneration bool   `json:"autoSBOMGeneration"`
	Hash               string `json:"hash"`
}

// DatasetMetadata is the persisted dataset contract for a run directory.
type DatasetMetadata struct {
	Profile     string             `json:"profile"`
	Policy      string             `json:"policy"`
	Fingerprint DatasetFingerprint `json:"fingerprint"`
	Contract    map[string]any     `json:"contract"`
}

// Fingerprint returns a DatasetFingerprint for the current config.
func (c *Config) Fingerprint() DatasetFingerprint {
	fp := DatasetFingerprint{
		SizePreset:         string(c.Size),
		ProjectPrefix:      c.ProjectPrefix,
		UserPrefix:         c.UserPrefix,
		Projects:           c.ProjectsCount,
		ReposPerProject:    c.RepositoriesCountPerProject,
		ArtifactsPerRepo:   c.ArtifactsCountPerRepository,
		TagsPerArtifact:    c.ArtifactTagsCountPerArtifact,
		Users:              c.UsersCount,
		MembersPerProject:  c.ProjectMembersCountPerProject,
		AuditLogs:          c.AuditLogsCount,
		BlobSize:           c.BlobSize,
		BlobsPerArtifact:   c.BlobsCountPerArtifact,
		ScannerURL:         c.ScannerURL,
		FakeScannerURL:     c.FakeScannerURL,
		AutoSBOMGeneration: c.AutoSBOMGeneration,
	}
	data := fmt.Sprintf("%s:%s:%s:%d:%d:%d:%d:%d:%d:%d:%s:%d:%s:%s:%t",
		fp.SizePreset, fp.ProjectPrefix, fp.UserPrefix,
		fp.Projects, fp.ReposPerProject, fp.ArtifactsPerRepo,
		fp.TagsPerArtifact, fp.Users, fp.MembersPerProject, fp.AuditLogs,
		fp.BlobSize, fp.BlobsPerArtifact, fp.ScannerURL, fp.FakeScannerURL, fp.AutoSBOMGeneration,
	)
	fp.Hash = fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
	return fp
}

// DatasetMetadata returns the dataset contract for this config.
func (c *Config) DatasetMetadata() DatasetMetadata {
	return DatasetMetadata{
		Profile:     string(c.Size),
		Policy:      string(c.DatasetPolicy),
		Fingerprint: c.Fingerprint(),
		Contract: map[string]any{
			"project_prefix":              c.ProjectPrefix,
			"user_prefix":                 c.UserPrefix,
			"projects":                    c.ProjectsCount,
			"repositories_per_project":    c.RepositoriesCountPerProject,
			"artifacts_per_repository":    c.ArtifactsCountPerRepository,
			"artifact_tags_per_artifact":  c.ArtifactTagsCountPerArtifact,
			"users":                       c.UsersCount,
			"project_members_per_project": c.ProjectMembersCountPerProject,
			"audit_logs":                  c.AuditLogsCount,
			"blob_size":                   c.BlobSize,
			"blobs_per_artifact":          c.BlobsCountPerArtifact,
			"scanner_url":                 c.ScannerURL,
			"fake_scanner_url":            c.FakeScannerURL,
			"auto_sbom_generation":        c.AutoSBOMGeneration,
		},
	}
}

// WriteDatasetJSON persists dataset metadata into the output directory.
func (c *Config) WriteDatasetJSON(outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c.DatasetMetadata(), "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(outputDir, "dataset.json"), data, 0644)
}

// Load reads configuration from environment variables with the same interface
// as the existing k6-based system.
func Load() (*Config, error) {
	cfg := &Config{
		ProjectPrefix:      getEnv("PROJECT_PREFIX", "project"),
		UserPrefix:         getEnv("USER_PREFIX", "user"),
		ScannerURL:         getEnv("SCANNER_URL", ""),
		FakeScannerURL:     getEnv("FAKE_SCANNER_URL", ""),
		AutoSBOMGeneration: getEnvBool("AUTO_SBOM_GENERATION"),
		OutputDir:          getEnv("HARBOR_OUTPUT_DIR", "./outputs"),
		CSVOutput:          getEnvBool("K6_CSV_OUTPUT"),
		JSONOutput:         getEnvBool("K6_JSON_OUTPUT"),
		ReportEnabled:      getEnvBool("HARBOR_REPORT"),
	}

	// Parse Harbor URL or individual components
	if harborURL := os.Getenv("HARBOR_URL"); harborURL != "" {
		u, err := url.Parse(harborURL)
		if err != nil {
			return nil, fmt.Errorf("invalid HARBOR_URL: %w", err)
		}
		cfg.Harbor.Scheme = u.Scheme
		cfg.Harbor.Host = u.Host
		if u.User != nil {
			cfg.Harbor.Username = u.User.Username()
			cfg.Harbor.Password, _ = u.User.Password()
		}
	} else {
		cfg.Harbor.Scheme = getEnv("HARBOR_SCHEME", "https")
		cfg.Harbor.Host = os.Getenv("HARBOR_HOST")
		cfg.Harbor.Username = getEnv("HARBOR_USERNAME", "admin")
		cfg.Harbor.Password = getEnv("HARBOR_PASSWORD", "Harbor12345")
	}

	if cfg.Harbor.Host == "" {
		return nil, fmt.Errorf("HARBOR_URL or HARBOR_HOST is required")
	}

	cfg.Harbor.Scheme = strings.ToLower(cfg.Harbor.Scheme)
	cfg.Harbor.Host = strings.TrimSuffix(cfg.Harbor.Host, "/")
	cfg.Harbor.Insecure = true // match existing behavior

	// Size preset
	sizeStr := getEnv("HARBOR_SIZE", "small")
	switch sizeStr {
	case "ci":
		cfg.Size = SizeCI
	case "small":
		cfg.Size = SizeSmall
	case "medium":
		cfg.Size = SizeMedium
	default:
		return nil, fmt.Errorf("unknown HARBOR_SIZE %q, must be ci/small/medium", sizeStr)
	}

	applySizePreset(cfg)

	// Override VUs/iterations from env if set
	if v := os.Getenv("HARBOR_VUS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid HARBOR_VUS: %w", err)
		}
		cfg.VUs = n
	}

	if v := os.Getenv("HARBOR_ITERATIONS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid HARBOR_ITERATIONS: %w", err)
		}
		cfg.Iterations = n
	}

	// Default iterations to 2x VUs if VUs set but iterations not
	if cfg.VUs > 0 && cfg.Iterations == 0 {
		cfg.Iterations = cfg.VUs * 2
	}

	// Dataset policy
	policyStr := getEnv("HARBOR_DATASET_POLICY", "reuse")
	switch policyStr {
	case "fresh":
		cfg.DatasetPolicy = PolicyFresh
	case "verify":
		cfg.DatasetPolicy = PolicyVerify
	case "reuse":
		cfg.DatasetPolicy = PolicyReuse
	default:
		return nil, fmt.Errorf("unknown HARBOR_DATASET_POLICY %q", policyStr)
	}

	return cfg, nil
}

func applySizePreset(cfg *Config) {
	switch cfg.Size {
	case SizeCI:
		cfg.VUs = getEnvIntDefault("HARBOR_VUS", 100)
		cfg.ProjectsCount = 10
		cfg.RepositoriesCountPerProject = 10
		cfg.ArtifactsCountPerRepository = 5
		cfg.ArtifactTagsCountPerArtifact = 5
		cfg.UsersCount = 10
		cfg.ProjectMembersCountPerProject = 5
		cfg.AuditLogsCount = 5000
		cfg.BlobSize = "1 KiB"
		cfg.BlobsCountPerArtifact = 1
	case SizeSmall:
		cfg.VUs = getEnvIntDefault("HARBOR_VUS", 300)
		cfg.ProjectsCount = 100
		cfg.RepositoriesCountPerProject = 100
		cfg.ArtifactsCountPerRepository = 10
		cfg.ArtifactTagsCountPerArtifact = 5
		cfg.UsersCount = 100
		cfg.ProjectMembersCountPerProject = 10
		cfg.AuditLogsCount = 100000
		cfg.BlobSize = getEnv("BLOB_SIZE", "1 KiB")
		cfg.BlobsCountPerArtifact = getEnvIntDefault("BLOBS_COUNT_PER_ARTIFACT", 1)
	case SizeMedium:
		cfg.VUs = getEnvIntDefault("HARBOR_VUS", 300)
		cfg.ProjectsCount = 200
		cfg.RepositoriesCountPerProject = 200
		cfg.ArtifactsCountPerRepository = 20
		cfg.ArtifactTagsCountPerArtifact = 10
		cfg.UsersCount = 200
		cfg.ProjectMembersCountPerProject = 20
		cfg.AuditLogsCount = 200000
		cfg.BlobSize = getEnv("BLOB_SIZE", "1 KiB")
		cfg.BlobsCountPerArtifact = getEnvIntDefault("BLOBS_COUNT_PER_ARTIFACT", 1)
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvBool(key string) bool {
	switch strings.ToLower(os.Getenv(key)) {
	case "true", "t", "yes", "y":
		return true
	default:
		return false
	}
}

func getEnvIntDefault(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}
