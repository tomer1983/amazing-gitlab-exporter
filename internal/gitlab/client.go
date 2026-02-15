package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/sirupsen/logrus"
	goGitlab "gitlab.com/gitlab-org/api/client-go"
)

// Client wraps a go-gitlab REST client, an optional GraphQL layer, and a
// rate limiter into a single entry-point for all GitLab API interactions.
type Client struct {
	rest        *goGitlab.Client
	rateLimiter *RateLimiter
	features    *DetectedFeatures
	logger      *logrus.Entry
	baseURL     string
	token       string
	useGraphQL  bool
}

// New creates a new Client configured against the given GitLab instance.
// rps and burst control the local token-bucket rate limiter (0 or negative
// disables it). useGraphQL enables the GraphQL transport for batch queries.
func New(baseURL, token string, rps, burst int, useGraphQL bool, logger *logrus.Entry) (*Client, error) {
	rest, err := goGitlab.NewClient(token, goGitlab.WithBaseURL(baseURL))
	if err != nil {
		return nil, fmt.Errorf("creating gitlab REST client: %w", err)
	}

	rl := NewRateLimiter(rps, burst, logger.WithField("component", "rate_limiter"))

	return &Client{
		rest:        rest,
		rateLimiter: rl,
		logger:      logger,
		baseURL:     baseURL,
		token:       token,
		useGraphQL:  useGraphQL,
	}, nil
}

// REST returns the underlying go-gitlab REST client.
func (c *Client) REST() *goGitlab.Client {
	return c.rest
}

// Features returns the detected GitLab tier features, or nil if detection
// has not been run yet.
func (c *Client) Features() *DetectedFeatures {
	return c.features
}

// SetFeatures stores the detected features on the client so that other
// components can inspect tier-dependent capabilities.
func (c *Client) SetFeatures(f *DetectedFeatures) {
	c.features = f
}

// RateLimiter returns the rate limiter associated with this client.
func (c *Client) RateLimiter() *RateLimiter {
	return c.rateLimiter
}

// BaseURL returns the base URL of the GitLab instance.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// DoREST performs a rate-limited REST call against the GitLab API.
// path is relative to the /api/v4/ root (e.g. "projects/1/pipelines").
// body is JSON-marshalled and sent as the request body if non-nil.
// result, if non-nil, is JSON-unmarshalled from the response body.
func (c *Client) DoREST(ctx context.Context, method, path string, body, result interface{}) (*goGitlab.Response, error) {
	// Wait for rate limiter before issuing the request.
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter wait: %w", err)
	}

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshalling request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := c.rest.NewRequest(method, path, bodyReader, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req = req.WithContext(ctx)

	resp, err := c.rest.Do(req, nil)
	if err != nil && resp == nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}

	// Always update rate limiter from response headers.
	if resp != nil {
		c.rateLimiter.UpdateFromHeaders(resp.Header)
	}

	if result != nil && resp != nil && resp.Body != nil {
		defer resp.Body.Close()
		if decErr := json.NewDecoder(resp.Body).Decode(result); decErr != nil {
			return resp, fmt.Errorf("decoding response: %w", decErr)
		}
	}

	return resp, err
}
