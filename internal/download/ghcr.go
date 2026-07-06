package download

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

type ghcrAuth struct {
	mu     sync.Mutex
	tokens map[string]string
	client *http.Client
}

func newGHCRAuth(client *http.Client) *ghcrAuth {
	return &ghcrAuth{
		tokens: make(map[string]string),
		client: client,
	}
}

func (g *ghcrAuth) token(ctx context.Context, blobURL string) (string, error) {
	repo, err := ghcrRepo(blobURL)
	if err != nil {
		return "", err
	}
	g.mu.Lock()
	if t, ok := g.tokens[repo]; ok {
		g.mu.Unlock()
		return t, nil
	}
	g.mu.Unlock()

	tokenURL := fmt.Sprintf("https://ghcr.io/token?service=ghcr.io&scope=repository:%s:pull", url.QueryEscape(repo))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "gale/1.0")
	resp, err := g.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("ghcr token %s: %s", repo, resp.Status)
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	g.mu.Lock()
	g.tokens[repo] = body.Token
	g.mu.Unlock()
	return body.Token, nil
}

func ghcrRepo(blobURL string) (string, error) {
	// https://ghcr.io/v2/homebrew/core/libpsl/blobs/sha256:...
	u, err := url.Parse(blobURL)
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 || parts[0] != "v2" || parts[len(parts)-2] != "blobs" {
		return "", fmt.Errorf("not a ghcr blob url: %s", blobURL)
	}
	return strings.Join(parts[1:len(parts)-2], "/"), nil
}

func isGHCR(raw string) bool {
	return strings.Contains(raw, "ghcr.io/")
}

func (d *Downloader) authorize(ctx context.Context, req *http.Request) error {
	if d.ghcr == nil {
		d.ghcr = newGHCRAuth(d.Client)
	}
	if !isGHCR(req.URL.String()) {
		return nil
	}
	tok, err := d.ghcr.token(ctx, req.URL.String())
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return nil
}
