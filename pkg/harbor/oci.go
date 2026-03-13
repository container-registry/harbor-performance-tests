package harbor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// OciDescriptor is an alias for the OCI descriptor type.
type OciDescriptor = ocispec.Descriptor

// PushOption configures an OCI push operation.
type PushOption struct {
	Ref   string
	Store *ContentStore
	Blobs []ocispec.Descriptor
}

// Push pushes an OCI artifact to Harbor.
func (c *Client) Push(ctx context.Context, option PushOption) (string, error) {
	ref := c.getRef(option.Ref)

	repo, err := c.newRemoteRepository(ref)
	if err != nil {
		return "", fmt.Errorf("create remote repository: %w", err)
	}

	tagOrDigest := extractTagOrDigest(ref)

	// Build config
	configBytes, _ := json.Marshal(map[string]interface{}{"User": uuid.New().String()})
	configDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageConfig,
		Digest:    digest.FromBytes(configBytes),
		Size:      int64(len(configBytes)),
	}

	// Use memory store to stage manifest
	memStore := memory.New()

	if err := memStore.Push(ctx, configDesc, bytes.NewReader(configBytes)); err != nil {
		return "", fmt.Errorf("push config: %w", err)
	}

	// Stage blobs from content store to memory store
	for _, blob := range option.Blobs {
		rc, err := option.Store.Store.Fetch(ctx, blob)
		if err != nil {
			return "", fmt.Errorf("fetch blob %s: %w", blob.Digest, err)
		}
		if err := memStore.Push(ctx, blob, rc); err != nil {
			rc.Close()
			return "", fmt.Errorf("stage blob %s: %w", blob.Digest, err)
		}
		rc.Close()
	}

	// Pack manifest
	manifestDesc, err := oras.PackManifest(ctx, memStore, oras.PackManifestVersion1_1, "",
		oras.PackManifestOptions{
			Layers:           option.Blobs,
			ConfigDescriptor: &configDesc,
		},
	)
	if err != nil {
		return "", fmt.Errorf("pack manifest: %w", err)
	}

	if err := memStore.Tag(ctx, manifestDesc, tagOrDigest); err != nil {
		return "", fmt.Errorf("tag manifest: %w", err)
	}

	manifestDesc, err = oras.Copy(ctx, memStore, tagOrDigest, repo, tagOrDigest, oras.DefaultCopyOptions)
	if err != nil {
		return "", fmt.Errorf("push %s: %w", ref, err)
	}

	return manifestDesc.Digest.String(), nil
}

// Pull pulls an OCI artifact from Harbor.
func (c *Client) Pull(ctx context.Context, ref string) error {
	ref = c.getRef(ref)

	repo, err := c.newRemoteRepository(ref)
	if err != nil {
		return fmt.Errorf("create remote repository: %w", err)
	}

	tagOrDigest := extractTagOrDigest(ref)
	dst := newDiscardStore()

	_, err = oras.Copy(ctx, repo, tagOrDigest, dst, tagOrDigest, oras.DefaultCopyOptions)
	if err != nil {
		return fmt.Errorf("pull %s: %w", ref, err)
	}
	return nil
}

func (c *Client) getRef(ref string) string {
	if !strings.HasPrefix(ref, c.Host) {
		return c.Host + "/" + ref
	}
	return ref
}

func (c *Client) newRemoteRepository(ref string) (*remote.Repository, error) {
	repo, err := remote.NewRepository(ref)
	if err != nil {
		return nil, err
	}

	transport := newTransport(c.Scheme != "https")

	repo.Client = &auth.Client{
		Client: &http.Client{Transport: transport},
		Credential: func(ctx context.Context, hostport string) (auth.Credential, error) {
			if hostport == c.Host {
				return auth.Credential{
					Username: c.Username,
					Password: c.Password,
				}, nil
			}
			return auth.EmptyCredential, nil
		},
	}

	repo.PlainHTTP = c.Scheme == "http"

	return repo, nil
}

func extractTagOrDigest(ref string) string {
	if idx := strings.LastIndex(ref, "@"); idx != -1 {
		return ref[idx+1:]
	}
	if idx := strings.LastIndex(ref, ":"); idx != -1 {
		candidate := ref[idx+1:]
		if !strings.Contains(candidate, "/") {
			return candidate
		}
	}
	return ref
}
