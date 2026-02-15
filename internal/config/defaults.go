package config

// ApplyDefaults sets sensible default values on the given Config.
// Values already set (non-zero) are not overwritten by YAML unmarshalling
// later, so these serve as the baseline configuration.
func ApplyDefaults(cfg *Config) {
	// --- Log ---
	cfg.Log.Level = "info"
	cfg.Log.Format = "text"

	// --- Server ---
	cfg.Server.ListenAddress = ":8080"

	// --- GitLab ---
	cfg.GitLab.URL = "https://gitlab.com"
	cfg.GitLab.EnableTLSVerify = true
	cfg.GitLab.MaxRequestsPerSecond = 10
	cfg.GitLab.BurstRequestsPerSecond = 20
	cfg.GitLab.UseGraphQL = true
	cfg.GitLab.GraphQLPageSize = 100
	cfg.GitLab.RESTPageSize = 100

	// --- Collectors ---

	// Pipelines
	cfg.Collectors.Pipelines.Enabled = true
	cfg.Collectors.Pipelines.IntervalSeconds = 30
	cfg.Collectors.Pipelines.IncludeChildPipelines = true
	cfg.Collectors.Pipelines.HistogramBuckets = []float64{5, 10, 30, 60, 120, 300, 600, 1800, 3600}
	cfg.Collectors.Pipelines.MaxPipelinesPerRef = 10

	// Jobs
	cfg.Collectors.Jobs.Enabled = true
	cfg.Collectors.Jobs.IntervalSeconds = 30
	cfg.Collectors.Jobs.HistogramBuckets = []float64{5, 10, 30, 60, 120, 300, 600, 1800}
	cfg.Collectors.Jobs.IncludeRunnerDetails = true

	// Merge Requests
	cfg.Collectors.MergeRequests.Enabled = true
	cfg.Collectors.MergeRequests.IntervalSeconds = 120
	cfg.Collectors.MergeRequests.HistogramBuckets = []float64{3600, 7200, 14400, 28800, 86400, 172800, 604800}

	// Environments
	cfg.Collectors.Environments.Enabled = false
	cfg.Collectors.Environments.IntervalSeconds = 300
	cfg.Collectors.Environments.ExcludeStopped = true

	// Test Reports
	cfg.Collectors.TestReports.Enabled = false
	cfg.Collectors.TestReports.IntervalSeconds = 60

	// DORA
	cfg.Collectors.DORA.Enabled = true
	cfg.Collectors.DORA.IntervalSeconds = 3600
	cfg.Collectors.DORA.EnvironmentTiers = []string{"production", "staging"}

	// Value Stream
	cfg.Collectors.ValueStream.Enabled = true
	cfg.Collectors.ValueStream.IntervalSeconds = 3600

	// Code Review
	cfg.Collectors.CodeReview.Enabled = true
	cfg.Collectors.CodeReview.IntervalSeconds = 300

	// Repository
	cfg.Collectors.Repository.Enabled = true
	cfg.Collectors.Repository.IntervalSeconds = 3600

	// Contributors
	cfg.Collectors.Contributors.Enabled = false
	cfg.Collectors.Contributors.IntervalSeconds = 3600

	// --- Project Defaults ---
	cfg.Defaults.OutputSparseStatusMetrics = true

	// Branches
	cfg.Defaults.Refs.Branches.Enabled = true
	cfg.Defaults.Refs.Branches.Regexp = `^(main|master|develop|release/.*)$`
	cfg.Defaults.Refs.Branches.ExcludeDeleted = true

	// Tags
	cfg.Defaults.Refs.Tags.Enabled = true
	cfg.Defaults.Refs.Tags.Regexp = `^v.*`
	cfg.Defaults.Refs.Tags.MostRecent = 10
	cfg.Defaults.Refs.Tags.MaxAgeDays = 90
	cfg.Defaults.Refs.Tags.ExcludeDeleted = true

	// Merge Request refs
	cfg.Defaults.Refs.MergeRequests.Enabled = true
	cfg.Defaults.Refs.MergeRequests.States = []string{"opened", "merged"}
	cfg.Defaults.Refs.MergeRequests.MostRecent = 20
	cfg.Defaults.Refs.MergeRequests.MaxAgeDays = 30
}
