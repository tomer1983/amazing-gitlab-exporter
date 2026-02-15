package gitlab

import (
	"context"
	"fmt"
	"net/http"
	"time"

	graphql "github.com/hasura/go-graphql-client"
)

// --------------------------------------------------------------------------
// GraphQL transport (adds the PRIVATE-TOKEN header)
// --------------------------------------------------------------------------

type tokenTransport struct {
	token string
	base  http.RoundTripper
}

func (t *tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("PRIVATE-TOKEN", t.token)
	return t.base.RoundTrip(req)
}

// --------------------------------------------------------------------------
// GraphQL client wrapper
// --------------------------------------------------------------------------

// graphQLClient wraps the hasura go-graphql-client with GitLab auth.
type graphQLClient struct {
	client *graphql.Client
}

// newGraphQLClient creates a GraphQL client targeting the given GitLab
// instance. The token is injected via a custom HTTP transport.
func newGraphQLClient(baseURL, token string) *graphQLClient {
	httpClient := &http.Client{
		Transport: &tokenTransport{
			token: token,
			base:  http.DefaultTransport,
		},
		Timeout: 30 * time.Second,
	}

	endpoint := baseURL + "/api/graphql"
	client := graphql.NewClient(endpoint, httpClient)

	return &graphQLClient{
		client: client,
	}
}

// --------------------------------------------------------------------------
// GraphQL response types
// --------------------------------------------------------------------------

// ProjectNode is the GraphQL representation of a GitLab project.
type ProjectNode struct {
	ID          string `graphql:"id"`
	FullPath    string `graphql:"fullPath"`
	Name        string `graphql:"name"`
	Description string `graphql:"description"`
	WebURL      string `graphql:"webUrl"`
	CreatedAt   string `graphql:"createdAt"`
}

// PipelineNode is the GraphQL representation of a GitLab CI/CD pipeline.
type PipelineNode struct {
	ID             string  `graphql:"id"`
	IID            string  `graphql:"iid"`
	Status         string  `graphql:"status"`
	Duration       *int    `graphql:"duration"`
	QueuedDuration *int    `graphql:"queuedDuration"`
	CreatedAt      string  `graphql:"createdAt"`
	FinishedAt     *string `graphql:"finishedAt"`
	Source         *string `graphql:"source"`
	Ref            *string `graphql:"ref"`
}

// MergeRequestNode is the GraphQL representation of a GitLab merge request.
type MergeRequestNode struct {
	IID          string  `graphql:"iid"`
	Title        string  `graphql:"title"`
	State        string  `graphql:"state"`
	CreatedAt    string  `graphql:"createdAt"`
	UpdatedAt    string  `graphql:"updatedAt"`
	MergedAt     *string `graphql:"mergedAt"`
	SourceBranch string  `graphql:"sourceBranch"`
	TargetBranch string  `graphql:"targetBranch"`
	WebURL       string  `graphql:"webUrl"`
}

// ProjectWithPipelines is a convenience type combining a project with its
// most recent pipelines, as returned by the batch GraphQL query.
type ProjectWithPipelines struct {
	Project   ProjectNode
	Pipelines []PipelineNode
}

// --------------------------------------------------------------------------
// GraphQL queries
// --------------------------------------------------------------------------

// FetchProjectsWithPipelines fetches multiple projects (by full path) along
// with their most recent pipelines in a single GraphQL request.
// first controls how many pipelines per project are returned.
func (c *Client) FetchProjectsWithPipelines(ctx context.Context, projectPaths []string, first int) ([]ProjectWithPipelines, error) {
	if !c.useGraphQL {
		return nil, fmt.Errorf("GraphQL is not enabled on this client")
	}

	gql := newGraphQLClient(c.baseURL, c.token)
	results := make([]ProjectWithPipelines, 0, len(projectPaths))

	// GraphQL doesn't natively support dynamic aliases in the hasura client,
	// so we query each project individually but reuse the same connection.
	for _, path := range projectPaths {
		if err := c.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter: %w", err)
		}

		var query struct {
			Project struct {
				ID          string `graphql:"id"`
				FullPath    string `graphql:"fullPath"`
				Name        string `graphql:"name"`
				Description string `graphql:"description"`
				WebURL      string `graphql:"webUrl"`
				CreatedAt   string `graphql:"createdAt"`
				Pipelines   struct {
					Nodes []PipelineNode `graphql:"nodes"`
				} `graphql:"pipelines(first: $first)"`
			} `graphql:"project(fullPath: $path)"`
		}

		variables := map[string]interface{}{
			"path":  graphql.String(path),
			"first": graphql.Int(first),
		}

		if err := gql.client.Query(ctx, &query, variables); err != nil {
			c.logger.WithField("project", path).WithError(err).
				Warn("GraphQL: failed to fetch project with pipelines")
			continue
		}

		results = append(results, ProjectWithPipelines{
			Project: ProjectNode{
				ID:          query.Project.ID,
				FullPath:    query.Project.FullPath,
				Name:        query.Project.Name,
				Description: query.Project.Description,
				WebURL:      query.Project.WebURL,
				CreatedAt:   query.Project.CreatedAt,
			},
			Pipelines: query.Project.Pipelines.Nodes,
		})
	}

	return results, nil
}

// FetchProjectMergeRequests fetches merge requests for a single project via
// GraphQL. state should be one of "opened", "closed", "merged", "all".
// first controls how many MRs are returned.
func (c *Client) FetchProjectMergeRequests(ctx context.Context, projectPath string, state string, first int) ([]MergeRequestNode, error) {
	if !c.useGraphQL {
		return nil, fmt.Errorf("GraphQL is not enabled on this client")
	}

	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	gql := newGraphQLClient(c.baseURL, c.token)

	var query struct {
		Project struct {
			MergeRequests struct {
				Nodes []MergeRequestNode `graphql:"nodes"`
			} `graphql:"mergeRequests(first: $first, state: $state)"`
		} `graphql:"project(fullPath: $path)"`
	}

	variables := map[string]interface{}{
		"path":  graphql.String(projectPath),
		"first": graphql.Int(first),
		"state": graphql.String(state),
	}

	if err := gql.client.Query(ctx, &query, variables); err != nil {
		return nil, fmt.Errorf("GraphQL: fetching MRs for %s: %w", projectPath, err)
	}

	return query.Project.MergeRequests.Nodes, nil
}
