// Package config provides configuration loading, validation, and defaults for
// the amazing-gitlab-exporter.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration for amazing-gitlab-exporter.
type Config struct {
	Log        LogConfig        `yaml:"log"        json:"log"`
	Server     ServerConfig     `yaml:"server"     json:"server"`
	Redis      RedisConfig      `yaml:"redis"      json:"redis"`
	GitLab     GitLabConfig     `yaml:"gitlab"     json:"gitlab"`
	Collectors CollectorsConfig `yaml:"collectors"  json:"collectors"`
	Defaults   ProjectDefaults  `yaml:"defaults"    json:"defaults"`
	Projects   []ProjectConfig  `yaml:"projects"    json:"projects"`
	Wildcards  []WildcardConfig `yaml:"wildcards"   json:"wildcards"`
}

// LogConfig holds logging configuration.
type LogConfig struct {
	Level  string `yaml:"level"  json:"level"  env:"AGE_LOG_LEVEL"  validate:"omitempty,oneof=trace debug info warn error fatal panic"`
	Format string `yaml:"format" json:"format" env:"AGE_LOG_FORMAT" validate:"omitempty,oneof=text json"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	ListenAddress string        `yaml:"listen_address" json:"listen_address" env:"AGE_LISTEN_ADDRESS" validate:"required"`
	EnablePprof   bool          `yaml:"enable_pprof"   json:"enable_pprof"   env:"AGE_ENABLE_PPROF"`
	Webhook       WebhookConfig `yaml:"webhook"        json:"webhook"`
}

// WebhookConfig holds webhook receiver settings.
type WebhookConfig struct {
	Enabled     bool   `yaml:"enabled"      json:"enabled"      env:"AGE_WEBHOOK_ENABLED"`
	SecretToken string `yaml:"secret_token" json:"secret_token" env:"AGE_WEBHOOK_SECRET_TOKEN"`
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	URL          string `yaml:"url"            json:"url"            env:"AGE_REDIS_URL"`
	PoolSize     int    `yaml:"pool_size"      json:"pool_size"      env:"AGE_REDIS_POOL_SIZE"      validate:"omitempty,min=1"`
	MinIdleConns int    `yaml:"min_idle_conns" json:"min_idle_conns" env:"AGE_REDIS_MIN_IDLE_CONNS"  validate:"omitempty,min=0"`
}

// GitLabConfig holds GitLab API connection settings.
type GitLabConfig struct {
	URL                    string `yaml:"url"                       json:"url"                       env:"AGE_GITLAB_URL"                  validate:"required,url"`
	Token                  string `yaml:"token"                     json:"token"                     env:"AGE_GITLAB_TOKEN"                validate:"required"`
	EnableTLSVerify        bool   `yaml:"enable_tls_verify"         json:"enable_tls_verify"         env:"AGE_GITLAB_ENABLE_TLS_VERIFY"`
	CACertPath             string `yaml:"ca_cert_path"              json:"ca_cert_path"              env:"AGE_GITLAB_CA_CERT_PATH"         validate:"omitempty,file"`
	MaxRequestsPerSecond   int    `yaml:"max_requests_per_second"   json:"max_requests_per_second"   env:"AGE_GITLAB_MAX_RPS"              validate:"omitempty,min=0"`
	BurstRequestsPerSecond int    `yaml:"burst_requests_per_second" json:"burst_requests_per_second" env:"AGE_GITLAB_BURST_RPS"            validate:"omitempty,min=0"`
	UseGraphQL             bool   `yaml:"use_graphql"               json:"use_graphql"               env:"AGE_GITLAB_USE_GRAPHQL"`
	GraphQLPageSize        int    `yaml:"graphql_page_size"         json:"graphql_page_size"         env:"AGE_GITLAB_GRAPHQL_PAGE_SIZE"    validate:"omitempty,min=1,max=100"`
	RESTPageSize           int    `yaml:"rest_page_size"            json:"rest_page_size"            env:"AGE_GITLAB_REST_PAGE_SIZE"       validate:"omitempty,min=1,max=100"`
}

// CollectorsConfig wraps individual collector configurations.
type CollectorsConfig struct {
	Pipelines     PipelinesCollectorConfig     `yaml:"pipelines"      json:"pipelines"`
	Jobs          JobsCollectorConfig          `yaml:"jobs"           json:"jobs"`
	MergeRequests MergeRequestsCollectorConfig `yaml:"merge_requests" json:"merge_requests"`
	Environments  EnvironmentsCollectorConfig  `yaml:"environments"   json:"environments"`
	TestReports   TestReportsCollectorConfig   `yaml:"test_reports"   json:"test_reports"`
	DORA          DORACollectorConfig          `yaml:"dora"           json:"dora"`
	ValueStream   ValueStreamCollectorConfig   `yaml:"value_stream"   json:"value_stream"`
	CodeReview    CodeReviewCollectorConfig    `yaml:"code_review"    json:"code_review"`
	Repository    RepositoryCollectorConfig    `yaml:"repository"     json:"repository"`
	Contributors  ContributorsCollectorConfig  `yaml:"contributors"   json:"contributors"`
}

// PipelinesCollectorConfig holds pipeline collector settings.
type PipelinesCollectorConfig struct {
	Enabled               bool      `yaml:"enabled"                json:"enabled"`
	IntervalSeconds       int       `yaml:"interval_seconds"       json:"interval_seconds"       validate:"omitempty,min=1"`
	IncludeChildPipelines bool      `yaml:"include_child_pipelines" json:"include_child_pipelines"`
	HistogramBuckets      []float64 `yaml:"histogram_buckets"      json:"histogram_buckets"`
	MaxPipelinesPerRef    int       `yaml:"max_pipelines_per_ref"  json:"max_pipelines_per_ref"  validate:"omitempty,min=1"`
}

// Interval returns the collector interval as a time.Duration.
func (c PipelinesCollectorConfig) Interval() time.Duration {
	return time.Duration(c.IntervalSeconds) * time.Second
}

// JobsCollectorConfig holds job collector settings.
type JobsCollectorConfig struct {
	Enabled              bool      `yaml:"enabled"               json:"enabled"`
	IntervalSeconds      int       `yaml:"interval_seconds"      json:"interval_seconds"       validate:"omitempty,min=1"`
	HistogramBuckets     []float64 `yaml:"histogram_buckets"     json:"histogram_buckets"`
	IncludeRunnerDetails bool      `yaml:"include_runner_details" json:"include_runner_details"`
}

// Interval returns the collector interval as a time.Duration.
func (c JobsCollectorConfig) Interval() time.Duration {
	return time.Duration(c.IntervalSeconds) * time.Second
}

// MergeRequestsCollectorConfig holds merge request collector settings.
type MergeRequestsCollectorConfig struct {
	Enabled          bool      `yaml:"enabled"          json:"enabled"`
	IntervalSeconds  int       `yaml:"interval_seconds" json:"interval_seconds" validate:"omitempty,min=1"`
	HistogramBuckets []float64 `yaml:"histogram_buckets" json:"histogram_buckets"`
}

// Interval returns the collector interval as a time.Duration.
func (c MergeRequestsCollectorConfig) Interval() time.Duration {
	return time.Duration(c.IntervalSeconds) * time.Second
}

// EnvironmentsCollectorConfig holds environment collector settings.
type EnvironmentsCollectorConfig struct {
	Enabled         bool `yaml:"enabled"          json:"enabled"`
	IntervalSeconds int  `yaml:"interval_seconds" json:"interval_seconds" validate:"omitempty,min=1"`
	ExcludeStopped  bool `yaml:"exclude_stopped"  json:"exclude_stopped"`
}

// Interval returns the collector interval as a time.Duration.
func (c EnvironmentsCollectorConfig) Interval() time.Duration {
	return time.Duration(c.IntervalSeconds) * time.Second
}

// TestReportsCollectorConfig holds test report collector settings.
type TestReportsCollectorConfig struct {
	Enabled          bool `yaml:"enabled"            json:"enabled"`
	IntervalSeconds  int  `yaml:"interval_seconds"   json:"interval_seconds" validate:"omitempty,min=1"`
	IncludeTestCases bool `yaml:"include_test_cases"  json:"include_test_cases"`
}

// Interval returns the collector interval as a time.Duration.
func (c TestReportsCollectorConfig) Interval() time.Duration {
	return time.Duration(c.IntervalSeconds) * time.Second
}

// DORACollectorConfig holds DORA metrics collector settings.
type DORACollectorConfig struct {
	Enabled          bool     `yaml:"enabled"           json:"enabled"`
	IntervalSeconds  int      `yaml:"interval_seconds"  json:"interval_seconds" validate:"omitempty,min=1"`
	EnvironmentTiers []string `yaml:"environment_tiers" json:"environment_tiers"`
}

// Interval returns the collector interval as a time.Duration.
func (c DORACollectorConfig) Interval() time.Duration {
	return time.Duration(c.IntervalSeconds) * time.Second
}

// ValueStreamCollectorConfig holds Value Stream Analytics collector settings.
type ValueStreamCollectorConfig struct {
	Enabled         bool `yaml:"enabled"          json:"enabled"`
	IntervalSeconds int  `yaml:"interval_seconds" json:"interval_seconds" validate:"omitempty,min=1"`
}

// Interval returns the collector interval as a time.Duration.
func (c ValueStreamCollectorConfig) Interval() time.Duration {
	return time.Duration(c.IntervalSeconds) * time.Second
}

// CodeReviewCollectorConfig holds Code Review Analytics collector settings.
type CodeReviewCollectorConfig struct {
	Enabled         bool `yaml:"enabled"          json:"enabled"`
	IntervalSeconds int  `yaml:"interval_seconds" json:"interval_seconds" validate:"omitempty,min=1"`
}

// Interval returns the collector interval as a time.Duration.
func (c CodeReviewCollectorConfig) Interval() time.Duration {
	return time.Duration(c.IntervalSeconds) * time.Second
}

// RepositoryCollectorConfig holds repository collector settings.
type RepositoryCollectorConfig struct {
	Enabled         bool `yaml:"enabled"          json:"enabled"`
	IntervalSeconds int  `yaml:"interval_seconds" json:"interval_seconds" validate:"omitempty,min=1"`
}

// Interval returns the collector interval as a time.Duration.
func (c RepositoryCollectorConfig) Interval() time.Duration {
	return time.Duration(c.IntervalSeconds) * time.Second
}

// ContributorsCollectorConfig holds contributor collector settings.
type ContributorsCollectorConfig struct {
	Enabled         bool `yaml:"enabled"          json:"enabled"`
	IntervalSeconds int  `yaml:"interval_seconds" json:"interval_seconds" validate:"omitempty,min=1"`
}

// Interval returns the collector interval as a time.Duration.
func (c ContributorsCollectorConfig) Interval() time.Duration {
	return time.Duration(c.IntervalSeconds) * time.Second
}

// ProjectDefaults holds default settings applied to all projects.
type ProjectDefaults struct {
	OutputSparseStatusMetrics bool       `yaml:"output_sparse_status_metrics" json:"output_sparse_status_metrics"`
	Refs                      RefsConfig `yaml:"refs"                         json:"refs"`
}

// RefsConfig holds Git ref filtering settings.
type RefsConfig struct {
	Branches      BranchesConfig         `yaml:"branches"       json:"branches"`
	Tags          TagsConfig             `yaml:"tags"           json:"tags"`
	MergeRequests MergeRequestsRefConfig `yaml:"merge_requests" json:"merge_requests"`
}

// BranchesConfig holds branch ref filter settings.
type BranchesConfig struct {
	Enabled        bool   `yaml:"enabled"        json:"enabled"`
	Regexp         string `yaml:"regexp"         json:"regexp"`
	MostRecent     int    `yaml:"most_recent"    json:"most_recent"    validate:"omitempty,min=0"`
	MaxAgeDays     int    `yaml:"max_age_days"   json:"max_age_days"   validate:"omitempty,min=0"`
	ExcludeDeleted bool   `yaml:"exclude_deleted" json:"exclude_deleted"`
}

// TagsConfig holds tag ref filter settings.
type TagsConfig struct {
	Enabled        bool   `yaml:"enabled"        json:"enabled"`
	Regexp         string `yaml:"regexp"         json:"regexp"`
	MostRecent     int    `yaml:"most_recent"    json:"most_recent"    validate:"omitempty,min=0"`
	MaxAgeDays     int    `yaml:"max_age_days"   json:"max_age_days"   validate:"omitempty,min=0"`
	ExcludeDeleted bool   `yaml:"exclude_deleted" json:"exclude_deleted"`
}

// MergeRequestsRefConfig holds merge request ref filter settings.
type MergeRequestsRefConfig struct {
	Enabled    bool     `yaml:"enabled"      json:"enabled"`
	States     []string `yaml:"states"       json:"states"`
	MostRecent int      `yaml:"most_recent"  json:"most_recent"  validate:"omitempty,min=0"`
	MaxAgeDays int      `yaml:"max_age_days" json:"max_age_days" validate:"omitempty,min=0"`
}

// ProjectConfig represents a single project to monitor.
type ProjectConfig struct {
	Name                      string      `yaml:"name"                         json:"name"                         validate:"required"`
	OutputSparseStatusMetrics *bool       `yaml:"output_sparse_status_metrics" json:"output_sparse_status_metrics"`
	Refs                      *RefsConfig `yaml:"refs"                         json:"refs"`
}

// WildcardConfig represents a dynamic project discovery rule.
type WildcardConfig struct {
	Owner    OwnerConfig `yaml:"owner"    json:"owner"    validate:"required"`
	Search   string      `yaml:"search"   json:"search"`
	Archived bool        `yaml:"archived" json:"archived"`
	Refs     *RefsConfig `yaml:"refs"     json:"refs"`
}

// OwnerConfig identifies the group or user that owns projects.
type OwnerConfig struct {
	Name             string `yaml:"name"              json:"name"              validate:"required"`
	Kind             string `yaml:"kind"              json:"kind"              validate:"required,oneof=group user"`
	IncludeSubgroups bool   `yaml:"include_subgroups" json:"include_subgroups"`
}

// Load reads a YAML configuration file, applies defaults, applies environment
// variable overrides, and validates the result.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := &Config{}
	ApplyDefaults(cfg)

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	applyEnvOverrides(cfg)

	if err := Validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// applyEnvOverrides walks the config struct and overwrites fields that have
// an "env" tag if the corresponding environment variable is set.
func applyEnvOverrides(cfg *Config) {
	applyEnvOverridesOnValue(reflect.ValueOf(cfg))
}

func applyEnvOverridesOnValue(v reflect.Value) {
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}

	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldVal := v.Field(i)

		// Recurse into embedded structs and struct fields.
		if fieldVal.Kind() == reflect.Struct {
			applyEnvOverridesOnValue(fieldVal.Addr())
			continue
		}
		if fieldVal.Kind() == reflect.Ptr && fieldVal.Type().Elem().Kind() == reflect.Struct {
			if !fieldVal.IsNil() {
				applyEnvOverridesOnValue(fieldVal)
			}
			continue
		}

		envKey := field.Tag.Get("env")
		if envKey == "" {
			continue
		}

		envVal, ok := os.LookupEnv(envKey)
		if !ok {
			continue
		}

		setFieldFromString(fieldVal, envVal)
	}
}

// setFieldFromString sets a reflect.Value from a string, supporting
// string, bool, int, float64, []string, and []float64 field types.
func setFieldFromString(field reflect.Value, raw string) {
	if !field.CanSet() {
		return
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(raw)

	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err == nil {
			field.SetBool(b)
		}

	case reflect.Int:
		n, err := strconv.Atoi(raw)
		if err == nil {
			field.SetInt(int64(n))
		}

	case reflect.Float64:
		f, err := strconv.ParseFloat(raw, 64)
		if err == nil {
			field.SetFloat(f)
		}

	case reflect.Slice:
		switch field.Type().Elem().Kind() {
		case reflect.String:
			parts := strings.Split(raw, ",")
			result := make([]string, 0, len(parts))
			for _, p := range parts {
				s := strings.TrimSpace(p)
				if s != "" {
					result = append(result, s)
				}
			}
			field.Set(reflect.ValueOf(result))

		case reflect.Float64:
			parts := strings.Split(raw, ",")
			result := make([]float64, 0, len(parts))
			for _, p := range parts {
				s := strings.TrimSpace(p)
				if s == "" {
					continue
				}
				f, err := strconv.ParseFloat(s, 64)
				if err == nil {
					result = append(result, f)
				}
			}
			field.Set(reflect.ValueOf(result))
		}
	}
}

// redactString replaces a secret string with "****" if non-empty.
func redactString(s string) string {
	if s == "" {
		return ""
	}
	return "****"
}

// Redacted returns a copy of the Config with sensitive fields masked.
func (c *Config) Redacted() Config {
	cp := *c
	cp.GitLab.Token = redactString(cp.GitLab.Token)
	cp.Server.Webhook.SecretToken = redactString(cp.Server.Webhook.SecretToken)
	cp.Redis.URL = redactString(cp.Redis.URL)
	return cp
}

// RedactedJSON returns the config as indented JSON with secrets masked.
func (c *Config) RedactedJSON() ([]byte, error) {
	redacted := c.Redacted()
	data, err := json.MarshalIndent(redacted, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshalling redacted config: %w", err)
	}
	return data, nil
}
