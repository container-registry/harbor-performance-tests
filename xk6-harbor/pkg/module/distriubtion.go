package module

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/grafana/sobek"
	"github.com/goharbor/xk6-harbor/pkg/util"
	"github.com/google/uuid"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"go.k6.io/k6/js/common"
)

type GetCatalogQuery struct {
	N    int    `js:"n"`
	Last string `js:"last"`
}

func (h *Harbor) GetCatalog(args ...sobek.Value) map[string]interface{} {
	h.mustInitialized()

	var param GetCatalogQuery
	if len(args) > 0 {
		rt := h.vu.Runtime()
		if err := rt.ExportTo(args[0], &param); err != nil {
			common.Throw(rt, err)
		}
	}

	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s://%s/v2/_catalog", h.option.Scheme, h.option.Host), nil)
	req.SetBasicAuth(h.option.Username, h.option.Password)

	q := req.URL.Query()
	if param.N != 0 {
		q.Add("n", strconv.Itoa(param.N))
	}

	if param.Last != "" {
		q.Add("last", param.Last)
	}

	req.URL.RawQuery = q.Encode()

	resp, err := h.httpClient.Do(req)
	Checkf(h.vu.Runtime(), err, "failed to get catalog")
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)

	m := map[string]interface{}{}
	Checkf(h.vu.Runtime(), dec.Decode(&m), "bad catalog")

	return m
}

func (h *Harbor) GetManifest(ref string) map[string]interface{} {
	h.mustInitialized()

	ref = h.getRef(ref)

	repo, err := h.newRemoteRepository(ref)
	Checkf(h.vu.Runtime(), err, "failed to create remote repository")

	// Extract the tag/digest from the full reference
	tagOrDigest := extractTagOrDigest(ref)

	desc, err := repo.Resolve(h.vu.Context(), tagOrDigest)
	Checkf(h.vu.Runtime(), err, "failed to head the manifest")

	rc, err := repo.Fetch(h.vu.Context(), desc)
	Checkf(h.vu.Runtime(), err, "failed to get the manifest")

	defer rc.Close()

	dec := json.NewDecoder(rc)

	m := map[string]interface{}{}
	Checkf(h.vu.Runtime(), dec.Decode(&m), "bad manifest")

	return m
}

type PullOption struct {
	Discard bool
}

func (h *Harbor) Pull(ref string, args ...sobek.Value) {
	h.mustInitialized()

	params := PullOption{}
	ExportTo(h.vu.Runtime(), &params, args...)

	ref = h.getRef(ref)

	repo, err := h.newRemoteRepository(ref)
	Checkf(h.vu.Runtime(), err, "failed to create remote repository")

	tagOrDigest := extractTagOrDigest(ref)

	var dst oras.Target
	if params.Discard {
		dst = newDiscardStore()
	} else {
		_, l := newLocalStore(h.vu.Runtime(), util.GenerateRandomString(8))
		dst = l
	}

	_, err = oras.Copy(h.vu.Context(), repo, tagOrDigest, dst, tagOrDigest, oras.DefaultCopyOptions)
	Checkf(h.vu.Runtime(), err, "failed to pull %s", ref)
}

type PushOption struct {
	Ref   string
	Store *ContentStore
	Blobs []ocispec.Descriptor
}

func (h *Harbor) Push(option PushOption, args ...sobek.Value) string {
	h.mustInitialized()

	ref := h.getRef(option.Ref)

	repo, err := h.newRemoteRepository(ref)
	Checkf(h.vu.Runtime(), err, "failed to create remote repository")

	tagOrDigest := extractTagOrDigest(ref)

	// Build config
	configBytes, _ := json.Marshal(map[string]interface{}{"User": uuid.New().String()})
	configDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageConfig,
		Digest:    digest.FromBytes(configBytes),
		Size:      int64(len(configBytes)),
	}

	// Use a memory store to stage the manifest
	memStore := memory.New()
	ctx := h.vu.Context()

	// Push config to memory store
	err = memStore.Push(ctx, configDesc, bytes.NewReader(configBytes))
	Checkf(h.vu.Runtime(), err, "failed to push config for %s", ref)

	// Push blobs from the file store to memory store
	for _, blob := range option.Blobs {
		rc, fetchErr := option.Store.Store.Fetch(ctx, blob)
		Checkf(h.vu.Runtime(), fetchErr, "failed to fetch blob %s", blob.Digest)
		pushErr := memStore.Push(ctx, blob, rc)
		rc.Close()
		Checkf(h.vu.Runtime(), pushErr, "failed to stage blob %s", blob.Digest)
	}

	// Pack a manifest
	manifestDesc, err := oras.PackManifest(ctx, memStore, oras.PackManifestVersion1_1, "",
		oras.PackManifestOptions{
			Layers:           option.Blobs,
			ConfigDescriptor: &configDesc,
		},
	)
	Checkf(h.vu.Runtime(), err, "failed to pack manifest for %s", ref)

	// Tag the manifest
	err = memStore.Tag(ctx, manifestDesc, tagOrDigest)
	Checkf(h.vu.Runtime(), err, "failed to tag manifest for %s", ref)

	// Copy from memory store to remote
	manifestDesc, err = oras.Copy(ctx, memStore, tagOrDigest, repo, tagOrDigest, oras.DefaultCopyOptions)
	Checkf(h.vu.Runtime(), err, "failed to push %s", ref)

	return manifestDesc.Digest.String()
}

func (h *Harbor) getRef(ref string) string {
	if !strings.HasPrefix(ref, h.option.Host) {
		return h.option.Host + "/" + ref
	}

	return ref
}

// newRemoteRepository creates a remote.Repository configured with auth and transport.
func (h *Harbor) newRemoteRepository(ref string) (*remote.Repository, error) {
	repo, err := remote.NewRepository(ref)
	if err != nil {
		return nil, err
	}

	var transport http.RoundTripper
	if h.option.Insecure {
		transport = util.NewInsecureTransport()
	} else {
		transport = util.NewDefaultTransport()
	}

	repo.Client = &auth.Client{
		Client: &http.Client{Transport: transport},
		Credential: func(ctx context.Context, hostport string) (auth.Credential, error) {
			if hostport == h.option.Host {
				return auth.Credential{
					Username: h.option.Username,
					Password: h.option.Password,
				}, nil
			}
			return auth.EmptyCredential, nil
		},
	}

	repo.PlainHTTP = h.option.Scheme == "http"

	return repo, nil
}

// extractTagOrDigest extracts the tag or digest portion from a full reference.
// e.g. "host/repo:tag" -> "tag", "host/repo@sha256:abc" -> "sha256:abc"
func extractTagOrDigest(ref string) string {
	if idx := strings.LastIndex(ref, "@"); idx != -1 {
		return ref[idx+1:]
	}
	if idx := strings.LastIndex(ref, ":"); idx != -1 {
		// Make sure this isn't a port separator by checking if there's a slash after
		candidate := ref[idx+1:]
		if !strings.Contains(candidate, "/") {
			return candidate
		}
	}
	return ref
}
