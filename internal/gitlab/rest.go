package gitlab

import (
	"context"
	"fmt"
	"time"

	goGitlab "gitlab.com/gitlab-org/api/client-go"
)

// --------------------------------------------------------------------------
// Pagination helper
// --------------------------------------------------------------------------

// fetchAllPages repeatedly calls fetchPage with incrementing page numbers
// until no more pages remain. fetchPage must populate its own result slice
// and return the go-gitlab Response (which carries pagination info) and any
// error.
func (c *Client) fetchAllPages(ctx context.Context, fetchPage func(page int) (*goGitlab.Response, error)) error {
	page := 1
	for {
		resp, err := fetchPage(page)
		if err != nil {
			return err
		}
		if resp == nil || resp.NextPage == 0 {
			return nil
		}
		page = resp.NextPage
	}
}

// --------------------------------------------------------------------------
// Projects
// --------------------------------------------------------------------------

// ListProjects returns all projects visible to the authenticated user that
// match the given options. Pagination is handled automatically.
func (c *Client) ListProjects(ctx context.Context, opts *goGitlab.ListProjectsOptions) ([]*goGitlab.Project, error) {
	var all []*goGitlab.Project

	if opts == nil {
		opts = &goGitlab.ListProjectsOptions{}
	}
	if opts.PerPage == 0 {
		opts.PerPage = 100
	}

	err := c.fetchAllPages(ctx, func(page int) (*goGitlab.Response, error) {
		opts.Page = page

		if err := c.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter: %w", err)
		}

		projects, resp, err := c.rest.Projects.ListProjects(opts)
		if err != nil {
			return resp, fmt.Errorf("listing projects (page %d): %w", page, err)
		}
		if resp != nil {
			c.rateLimiter.UpdateFromHeaders(resp.Header)
		}

		all = append(all, projects...)
		return resp, nil
	})

	return all, err
}

// GetProject fetches a single project by ID.
func (c *Client) GetProject(ctx context.Context, projectID int) (*goGitlab.Project, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	project, resp, err := c.rest.Projects.GetProject(projectID, nil)
	if resp != nil {
		c.rateLimiter.UpdateFromHeaders(resp.Header)
	}
	if err != nil {
		return nil, fmt.Errorf("getting project %d: %w", projectID, err)
	}

	return project, nil
}

// --------------------------------------------------------------------------
// Pipelines
// --------------------------------------------------------------------------

// ListPipelinesOptions extends the go-gitlab options with commonly used
// filters that the exporter needs.
type ListPipelinesOptions struct {
	*goGitlab.ListProjectPipelinesOptions
}

// ListPipelines returns pipelines for a project matching the given filters.
// Pagination is handled automatically.
func (c *Client) ListPipelines(ctx context.Context, projectID int, opts *goGitlab.ListProjectPipelinesOptions) ([]*goGitlab.PipelineInfo, error) {
	var all []*goGitlab.PipelineInfo

	if opts == nil {
		opts = &goGitlab.ListProjectPipelinesOptions{}
	}
	if opts.PerPage == 0 {
		opts.PerPage = 100
	}

	err := c.fetchAllPages(ctx, func(page int) (*goGitlab.Response, error) {
		opts.Page = page

		if err := c.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter: %w", err)
		}

		pipelines, resp, err := c.rest.Pipelines.ListProjectPipelines(projectID, opts)
		if err != nil {
			return resp, fmt.Errorf("listing pipelines for project %d (page %d): %w", projectID, page, err)
		}
		if resp != nil {
			c.rateLimiter.UpdateFromHeaders(resp.Header)
		}

		all = append(all, pipelines...)
		return resp, nil
	})

	return all, err
}

// GetPipeline fetches the full details of a single pipeline.
func (c *Client) GetPipeline(ctx context.Context, projectID, pipelineID int) (*goGitlab.Pipeline, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	pipeline, resp, err := c.rest.Pipelines.GetPipeline(projectID, pipelineID)
	if resp != nil {
		c.rateLimiter.UpdateFromHeaders(resp.Header)
	}
	if err != nil {
		return nil, fmt.Errorf("getting pipeline %d/%d: %w", projectID, pipelineID, err)
	}

	return pipeline, nil
}

// --------------------------------------------------------------------------
// Jobs
// --------------------------------------------------------------------------

// ListPipelineJobs returns all jobs for a given pipeline, paginating
// automatically.
func (c *Client) ListPipelineJobs(ctx context.Context, projectID, pipelineID int) ([]*goGitlab.Job, error) {
	var all []*goGitlab.Job

	opts := &goGitlab.ListJobsOptions{
		ListOptions: goGitlab.ListOptions{PerPage: 100},
	}

	err := c.fetchAllPages(ctx, func(page int) (*goGitlab.Response, error) {
		opts.Page = page

		if err := c.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter: %w", err)
		}

		jobs, resp, err := c.rest.Jobs.ListPipelineJobs(projectID, pipelineID, opts)
		if err != nil {
			return resp, fmt.Errorf("listing jobs for pipeline %d/%d (page %d): %w", projectID, pipelineID, page, err)
		}
		if resp != nil {
			c.rateLimiter.UpdateFromHeaders(resp.Header)
		}

		all = append(all, jobs...)
		return resp, nil
	})

	return all, err
}

// ListPipelineBridges returns all bridge (trigger) jobs for a pipeline.
// These are used to discover child/downstream pipelines.
func (c *Client) ListPipelineBridges(ctx context.Context, projectID, pipelineID int) ([]*goGitlab.Bridge, error) {
	var all []*goGitlab.Bridge

	opts := &goGitlab.ListJobsOptions{
		ListOptions: goGitlab.ListOptions{PerPage: 100},
	}

	err := c.fetchAllPages(ctx, func(page int) (*goGitlab.Response, error) {
		opts.Page = page

		if err := c.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter: %w", err)
		}

		bridges, resp, err := c.rest.Jobs.ListPipelineBridges(projectID, pipelineID, opts)
		if err != nil {
			return resp, fmt.Errorf("listing bridges for pipeline %d/%d (page %d): %w", projectID, pipelineID, page, err)
		}
		if resp != nil {
			c.rateLimiter.UpdateFromHeaders(resp.Header)
		}

		all = append(all, bridges...)
		return resp, nil
	})

	return all, err
}

// --------------------------------------------------------------------------
// Test reports
// --------------------------------------------------------------------------

// GetPipelineTestReport fetches the test report summary for a pipeline.
func (c *Client) GetPipelineTestReport(ctx context.Context, projectID, pipelineID int) (*goGitlab.PipelineTestReport, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	report, resp, err := c.rest.Pipelines.GetPipelineTestReport(projectID, pipelineID)
	if resp != nil {
		c.rateLimiter.UpdateFromHeaders(resp.Header)
	}
	if err != nil {
		return nil, fmt.Errorf("getting test report for pipeline %d/%d: %w", projectID, pipelineID, err)
	}

	return report, nil
}

// --------------------------------------------------------------------------
// Merge Requests
// --------------------------------------------------------------------------

// ListMergeRequests returns merge requests for a project matching the given
// filters. Pagination is handled automatically.
func (c *Client) ListMergeRequests(ctx context.Context, projectID int, opts *goGitlab.ListProjectMergeRequestsOptions) ([]*goGitlab.MergeRequest, error) {
	var all []*goGitlab.MergeRequest

	if opts == nil {
		opts = &goGitlab.ListProjectMergeRequestsOptions{}
	}
	if opts.PerPage == 0 {
		opts.PerPage = 100
	}

	err := c.fetchAllPages(ctx, func(page int) (*goGitlab.Response, error) {
		opts.Page = page

		if err := c.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter: %w", err)
		}

		mrs, resp, err := c.rest.MergeRequests.ListProjectMergeRequests(projectID, opts)
		if err != nil {
			return resp, fmt.Errorf("listing merge requests for project %d (page %d): %w", projectID, page, err)
		}
		if resp != nil {
			c.rateLimiter.UpdateFromHeaders(resp.Header)
		}

		all = append(all, mrs...)
		return resp, nil
	})

	return all, err
}

// GetMergeRequest fetches a single merge request by IID.
func (c *Client) GetMergeRequest(ctx context.Context, projectID, mrIID int) (*goGitlab.MergeRequest, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	mr, resp, err := c.rest.MergeRequests.GetMergeRequest(projectID, mrIID, nil)
	if resp != nil {
		c.rateLimiter.UpdateFromHeaders(resp.Header)
	}
	if err != nil {
		return nil, fmt.Errorf("getting MR %d in project %d: %w", mrIID, projectID, err)
	}

	return mr, nil
}

// --------------------------------------------------------------------------
// Environments & Deployments
// --------------------------------------------------------------------------

// ListEnvironments returns all environments for the given project.
func (c *Client) ListEnvironments(ctx context.Context, projectID int) ([]*goGitlab.Environment, error) {
	var all []*goGitlab.Environment

	opts := &goGitlab.ListEnvironmentsOptions{
		ListOptions: goGitlab.ListOptions{PerPage: 100},
	}

	err := c.fetchAllPages(ctx, func(page int) (*goGitlab.Response, error) {
		opts.Page = page

		if err := c.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter: %w", err)
		}

		envs, resp, err := c.rest.Environments.ListEnvironments(projectID, opts)
		if err != nil {
			return resp, fmt.Errorf("listing environments for project %d (page %d): %w", projectID, page, err)
		}
		if resp != nil {
			c.rateLimiter.UpdateFromHeaders(resp.Header)
		}

		all = append(all, envs...)
		return resp, nil
	})

	return all, err
}

// ListDeployments returns deployments for a project, optionally filtered by
// environment name.
func (c *Client) ListDeployments(ctx context.Context, projectID int, environmentName string) ([]*goGitlab.Deployment, error) {
	var all []*goGitlab.Deployment

	opts := &goGitlab.ListProjectDeploymentsOptions{
		ListOptions: goGitlab.ListOptions{PerPage: 100},
	}
	if environmentName != "" {
		opts.Environment = goGitlab.Ptr(environmentName)
	}

	err := c.fetchAllPages(ctx, func(page int) (*goGitlab.Response, error) {
		opts.Page = page

		if err := c.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter: %w", err)
		}

		deps, resp, err := c.rest.Deployments.ListProjectDeployments(projectID, opts)
		if err != nil {
			return resp, fmt.Errorf("listing deployments for project %d (page %d): %w", projectID, page, err)
		}
		if resp != nil {
			c.rateLimiter.UpdateFromHeaders(resp.Header)
		}

		all = append(all, deps...)
		return resp, nil
	})

	return all, err
}

// --------------------------------------------------------------------------
// Repository
// --------------------------------------------------------------------------

// GetRepositoryLanguages returns the language breakdown for a project.
func (c *Client) GetRepositoryLanguages(ctx context.Context, projectID int) (map[string]float32, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	languages, resp, err := c.rest.Projects.GetProjectLanguages(projectID)
	if resp != nil {
		c.rateLimiter.UpdateFromHeaders(resp.Header)
	}
	if err != nil {
		return nil, fmt.Errorf("getting languages for project %d: %w", projectID, err)
	}

	result := make(map[string]float32, len(*languages))
	for lang, pct := range *languages {
		result[lang] = pct
	}

	return result, nil
}

// ListCommits returns commits for a project on the given ref.
func (c *Client) ListCommits(ctx context.Context, projectID int, ref string) ([]*goGitlab.Commit, error) {
	var all []*goGitlab.Commit

	opts := &goGitlab.ListCommitsOptions{
		ListOptions: goGitlab.ListOptions{PerPage: 100},
	}
	if ref != "" {
		opts.RefName = goGitlab.Ptr(ref)
	}

	err := c.fetchAllPages(ctx, func(page int) (*goGitlab.Response, error) {
		opts.Page = page

		if err := c.rateLimiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter: %w", err)
		}

		commits, resp, err := c.rest.Commits.ListCommits(projectID, opts)
		if err != nil {
			return resp, fmt.Errorf("listing commits for project %d (page %d): %w", projectID, page, err)
		}
		if resp != nil {
			c.rateLimiter.UpdateFromHeaders(resp.Header)
		}

		all = append(all, commits...)
		return resp, nil
	})

	return all, err
}

// --------------------------------------------------------------------------
// DORA Metrics
// --------------------------------------------------------------------------

// DORAMetric represents a single DORA metric data point returned by the
// GitLab DORA API. We define this locally because the go-gitlab library
// may not yet have a dedicated type.
type DORAMetric struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
}

// GetDORAMetrics fetches DORA metrics for a project in the given date range.
// metric should be one of: deployment_frequency, lead_time_for_changes,
// time_to_restore_service, change_failure_rate.
func (c *Client) GetDORAMetrics(ctx context.Context, projectID int, metric string, startDate, endDate time.Time) ([]DORAMetric, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	path := fmt.Sprintf("projects/%d/dora/metrics?metric=%s&start_date=%s&end_date=%s",
		projectID, metric,
		startDate.Format("2006-01-02"),
		endDate.Format("2006-01-02"),
	)

	var metrics []DORAMetric
	req, err := c.rest.NewRequest("GET", path, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("building DORA metrics request: %w", err)
	}
	req = req.WithContext(ctx)

	resp, err := c.rest.Do(req, &metrics)
	if resp != nil {
		c.rateLimiter.UpdateFromHeaders(resp.Header)
	}
	if err != nil {
		return nil, fmt.Errorf("fetching DORA metric %q for project %d: %w", metric, projectID, err)
	}

	return metrics, nil
}

// --------------------------------------------------------------------------
// Project Statistics
// --------------------------------------------------------------------------

// GetProjectStatistics fetches a project with statistics included (sizes, etc.).
func (c *Client) GetProjectStatistics(ctx context.Context, projectID int) (*goGitlab.Project, error) {
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	opts := &goGitlab.GetProjectOptions{
		Statistics: goGitlab.Ptr(true),
	}

	project, resp, err := c.rest.Projects.GetProject(projectID, opts)
	if resp != nil {
		c.rateLimiter.UpdateFromHeaders(resp.Header)
	}
	if err != nil {
		return nil, fmt.Errorf("getting statistics for project %d: %w", projectID, err)
	}

	return project, nil
}
