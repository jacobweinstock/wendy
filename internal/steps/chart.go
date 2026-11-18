package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	ghcrHost            = "ghcr.io"
	tinkerbellImageRepo = "tinkerbell/tinkerbell"
	tinkerbellChartRepo = "tinkerbell/charts/tinkerbell"
	// Bounds the walk if GHCR ever stops listing tags in push order.
	maxVersionLookups = 50
)

// Releases are vX.Y.Z; other pushes add the short commit, newer ones with a
// "g" as git describe does.
var versionTag = regexp.MustCompile(`^v\d+\.\d+\.\d+(-g?[0-9a-f]{7,40})?$`)

var manifestAccept = strings.Join([]string{
	"application/vnd.oci.image.index.v1+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.docker.distribution.manifest.v2+json",
}, ", ")

var latest struct {
	mu          sync.Mutex
	byExecution map[string]string
}

// chartVersion resolves "latest" once per execution, so every combination of
// an execution installs the same chart even if main moves meanwhile.
func chartVersion(ctx context.Context, execution, location, version string) (string, error) {
	if version != "latest" {
		return version, nil
	}
	if location != "oci://ghcr.io/tinkerbell/charts/tinkerbell" {
		return "", fmt.Errorf("\"latest\" can only be resolved for the upstream chart; set chart.version for %s", location)
	}
	latest.mu.Lock()
	defer latest.mu.Unlock()
	if v, ok := latest.byExecution[execution]; ok {
		return v, nil
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	v, err := newGHCR().latestChartVersion(ctx)
	if err != nil {
		return "", fmt.Errorf("resolving the latest Tinkerbell chart: %w; set chart.version to name one", err)
	}
	if latest.byExecution == nil {
		latest.byExecution = map[string]string{}
	}
	latest.byExecution[execution] = v
	return v, nil
}

// ImageExists reports whether a public ghcr.io image tag exists, such as
// the OS image a testbed's machines will stream to disk.
func ImageExists(ctx context.Context, ref string) (bool, error) {
	rest, ok := strings.CutPrefix(ref, ghcrHost+"/")
	if !ok {
		return false, fmt.Errorf("only %s images can be checked: %s", ghcrHost, ref)
	}
	repo, tag, ok := strings.Cut(rest, ":")
	if !ok {
		return false, fmt.Errorf("no tag in %s", ref)
	}
	d, err := newGHCR().digest(ctx, repo, tag)
	return d != "", err
}

// OSImage is the OS image a testbed's machines stream to disk.
func OSImage(osVersion, kubernetesVersion, arch string) string {
	return fmt.Sprintf("%s:%s-%s-%s.gz", osRegistry, strings.ReplaceAll(osVersion, ".", ""), kubernetesVersion, arch)
}

// latestChartVersion is the chart version `latest` points at. The chart has
// no `latest` tag, so it is read off the image that does: the version tag
// sharing its digest, confirmed as published for the chart too.
func (g *ghcr) latestChartVersion(ctx context.Context) (string, error) {
	want, err := g.digest(ctx, tinkerbellImageRepo, "latest")
	if err != nil {
		return "", err
	}
	if want == "" {
		return "", fmt.Errorf("%s/%s has no latest tag", ghcrHost, tinkerbellImageRepo)
	}
	tags, err := g.tags(ctx, tinkerbellImageRepo)
	if err != nil {
		return "", err
	}
	tried := 0
	for i := len(tags) - 1; i >= 0 && tried < maxVersionLookups; i-- {
		tag := tags[i]
		if !versionTag.MatchString(tag) {
			continue
		}
		tried++
		got, err := g.digest(ctx, tinkerbellImageRepo, tag)
		if err != nil {
			return "", err
		}
		if got != want {
			continue
		}
		// The artifacts are published one after another; an execution in
		// that window would otherwise fail much later, at helm install.
		chart, err := g.digest(ctx, tinkerbellChartRepo, tag)
		if err != nil {
			return "", err
		}
		if chart == "" {
			return "", fmt.Errorf("%s/%s:%s is what latest points at, but its chart is not published yet", ghcrHost, tinkerbellImageRepo, tag)
		}
		return tag, nil
	}
	return "", fmt.Errorf("no version tag on %s/%s shares a digest with latest (tried %d of %d tags)", ghcrHost, tinkerbellImageRepo, tried, len(tags))
}

// ghcr reads tags and digests of public repositories with the anonymous
// pull token GHCR issues to anyone.
type ghcr struct {
	client *http.Client
	base   string
	mu     sync.Mutex
	tokens map[string]string
}

func newGHCR() *ghcr {
	return &ghcr{client: &http.Client{Timeout: 15 * time.Second}, base: "https://" + ghcrHost, tokens: map[string]string{}}
}

// digest is the manifest digest of repo:ref, or "" when there is no such tag.
func (g *ghcr) digest(ctx context.Context, repo, ref string) (string, error) {
	resp, err := g.do(ctx, http.MethodHead, repo, g.base+"/v2/"+repo+"/manifests/"+url.PathEscape(ref), manifestAccept)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		d := resp.Header.Get("Docker-Content-Digest")
		if d == "" {
			return "", fmt.Errorf("%s/%s:%s: no digest in the response", ghcrHost, repo, ref)
		}
		return d, nil
	case http.StatusNotFound:
		return "", nil
	}
	return "", fmt.Errorf("%s/%s:%s: %s", ghcrHost, repo, ref, resp.Status)
}

func (g *ghcr) tags(ctx context.Context, repo string) ([]string, error) {
	var all []string
	next := g.base + "/v2/" + repo + "/tags/list?n=1000"
	for next != "" {
		resp, err := g.do(ctx, http.MethodGet, repo, next, "application/json")
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("listing tags for %s/%s: %s", ghcrHost, repo, resp.Status)
		}
		var page struct {
			Tags []string `json:"tags"`
		}
		err = json.NewDecoder(resp.Body).Decode(&page)
		link := resp.Header.Get("Link")
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		all = append(all, page.Tags...)
		next = g.nextPage(link)
	}
	if len(all) == 0 {
		return nil, fmt.Errorf("%s/%s has no tags", ghcrHost, repo)
	}
	return all, nil
}

// nextPage is the URL in a `Link: <...>; rel="next"` header.
func (g *ghcr) nextPage(header string) string {
	for _, part := range strings.Split(header, ",") {
		target, params, ok := strings.Cut(strings.TrimSpace(part), ">")
		if !ok || !strings.Contains(params, `rel="next"`) {
			continue
		}
		target = strings.TrimPrefix(target, "<")
		if strings.HasPrefix(target, "/") {
			return g.base + target
		}
		return target
	}
	return ""
}

func (g *ghcr) do(ctx context.Context, method, repo, rawURL, accept string) (*http.Response, error) {
	token, err := g.token(ctx, repo)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", accept)
	return g.client.Do(req)
}

func (g *ghcr) token(ctx context.Context, repo string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if t, ok := g.tokens[repo]; ok {
		return t, nil
	}
	q := url.Values{"service": {ghcrHost}, "scope": {"repository:" + repo + ":pull"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.base+"/token?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting a pull token for %s/%s: %w", ghcrHost, repo, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("requesting a pull token for %s/%s: %s", ghcrHost, repo, resp.Status)
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.Token == "" {
		return "", fmt.Errorf("%s/%s: empty pull token", ghcrHost, repo)
	}
	g.tokens[repo] = body.Token
	return body.Token, nil
}
