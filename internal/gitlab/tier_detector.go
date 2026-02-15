package gitlab

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// DetectedFeatures describes the GitLab instance capabilities determined
// by probing various API endpoints. It is populated by Client.DetectFeatures.
type DetectedFeatures struct {
	// HasDORA is true when the DORA metrics API is available (Ultimate tier).
	HasDORA bool
	// HasValueStream is true when Value Stream Analytics endpoints respond (Premium+).
	HasValueStream bool
	// HasMRAnalytics is true when the MR analytics endpoint is available (Premium+).
	HasMRAnalytics bool
	// HasCodeReview is true when Code Review Analytics are available (Premium+).
	HasCodeReview bool
	// Tier represents the detected GitLab tier: 0=Free, 1=Premium, 2=Ultimate.
	Tier int
	// GitLabVersion is the version reported by /api/v4/version.
	GitLabVersion string
}

// TierFree, TierPremium, and TierUltimate are convenience constants for
// DetectedFeatures.Tier.
const (
	TierFree    = 0
	TierPremium = 1
	TierUltimate = 2
)

// TierDetector probes a GitLab instance to discover available features and
// the effective licence tier. It uses the REST client from *Client directly.
type TierDetector struct {
	rest   *gitlab.Client
	logger *logrus.Entry
}

// NewTierDetector returns a new detector backed by the given REST client.
func NewTierDetector(rest *gitlab.Client, logger *logrus.Entry) *TierDetector {
	return &TierDetector{
		rest:   rest,
		logger: logger,
	}
}

// Detect probes the GitLab instance and returns the detected features.
// It starts with a /api/v4/version call to verify connectivity, then
// progressively probes tier-specific endpoints.
func (td *TierDetector) Detect(ctx context.Context) (*DetectedFeatures, error) {
	features := &DetectedFeatures{}

	// --- Step 1: Version check (connectivity) ---
	version, err := td.checkVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("tier detection: version check failed: %w", err)
	}
	features.GitLabVersion = version
	td.logger.WithField("version", version).Info("connected to GitLab instance")

	// --- Step 2: Probe Ultimate-tier endpoints (DORA) ---
	features.HasDORA = td.probeDORA(ctx)
	if features.HasDORA {
		td.logger.Info("DORA metrics available (Ultimate tier detected)")
	}

	// --- Step 3: Probe Premium-tier endpoints ---
	features.HasValueStream = td.probeValueStream(ctx)
	features.HasMRAnalytics = td.probeMRAnalytics(ctx)
	features.HasCodeReview = td.probeCodeReview(ctx)

	// --- Step 4: Derive tier ---
	features.Tier = td.deriveTier(features)
	td.logger.WithFields(logrus.Fields{
		"tier":           tierName(features.Tier),
		"dora":           features.HasDORA,
		"value_stream":   features.HasValueStream,
		"mr_analytics":   features.HasMRAnalytics,
		"code_review":    features.HasCodeReview,
	}).Info("tier detection completed")

	return features, nil
}

// checkVersion calls GET /api/v4/version and returns the version string.
func (td *TierDetector) checkVersion(ctx context.Context) (string, error) {
	v, resp, err := td.rest.Version.GetVersion()
	if err != nil {
		return "", fmt.Errorf("GET /version: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET /version: unexpected status %d", resp.StatusCode)
	}
	return v.Version, nil
}

// probeDORA tries the DORA metrics endpoint with a dummy project query.
// A 200 response (even if empty) means the feature is available.
func (td *TierDetector) probeDORA(ctx context.Context) bool {
	// Use project ID 0 – we expect a 404 or empty result on success, but
	// if the endpoint itself returns 403 the feature is licence-gated.
	// Instead, we query the groups DORA endpoint which is more reliable
	// for detection. We fall back to a simple REST request.
	path := "projects/0/dora/metrics"
	req, err := td.rest.NewRequest(http.MethodGet, path, nil, nil)
	if err != nil {
		td.logger.WithError(err).Debug("DORA probe: failed to build request")
		return false
	}

	resp, err := td.rest.Do(req, nil)
	if err != nil {
		// Network errors are not feature-related.
		if resp == nil {
			td.logger.WithError(err).Debug("DORA probe: network error")
			return false
		}
	}

	// 403 → licence required; 404 → endpoint exists (project doesn't);
	// 400/422 → parameter issues but endpoint exists.
	switch {
	case resp.StatusCode == http.StatusForbidden:
		return false
	case resp.StatusCode == http.StatusNotFound,
		resp.StatusCode == http.StatusOK,
		resp.StatusCode == http.StatusBadRequest,
		resp.StatusCode == http.StatusUnprocessableEntity:
		return true
	default:
		td.logger.WithField("status", resp.StatusCode).Debug("DORA probe: unexpected status")
		return false
	}
}

// probeValueStream checks if Value Stream Analytics endpoints are reachable.
func (td *TierDetector) probeValueStream(ctx context.Context) bool {
	path := "projects/0/analytics/value_stream_analytics/stages"
	return td.probeEndpoint(ctx, path, "value_stream")
}

// probeMRAnalytics checks if MR Analytics endpoints are reachable.
func (td *TierDetector) probeMRAnalytics(ctx context.Context) bool {
	path := "projects/0/analytics/merge_request_analytics"
	return td.probeEndpoint(ctx, path, "mr_analytics")
}

// probeCodeReview checks if Code Review Analytics endpoints are reachable.
func (td *TierDetector) probeCodeReview(ctx context.Context) bool {
	path := "projects/0/analytics/code_review"
	return td.probeEndpoint(ctx, path, "code_review")
}

// probeEndpoint performs a GET against the given API path and returns true
// if the endpoint exists (i.e., the response is anything other than 403).
func (td *TierDetector) probeEndpoint(_ context.Context, path, label string) bool {
	req, err := td.rest.NewRequest(http.MethodGet, path, nil, nil)
	if err != nil {
		td.logger.WithError(err).Debugf("%s probe: failed to build request", label)
		return false
	}

	resp, err := td.rest.Do(req, nil)
	if err != nil && resp == nil {
		td.logger.WithError(err).Debugf("%s probe: network error", label)
		return false
	}

	// 403 means the feature is gated behind a higher tier.
	if resp.StatusCode == http.StatusForbidden {
		return false
	}
	return true
}

// deriveTier picks the highest tier supported by the detected features.
func (td *TierDetector) deriveTier(f *DetectedFeatures) int {
	if f.HasDORA {
		return TierUltimate
	}
	if f.HasValueStream || f.HasMRAnalytics || f.HasCodeReview {
		return TierPremium
	}
	return TierFree
}

// tierName returns a human-readable name for the tier constant.
func tierName(tier int) string {
	switch tier {
	case TierPremium:
		return "Premium"
	case TierUltimate:
		return "Ultimate"
	default:
		return "Free"
	}
}

// lastUpdatedTimes is a small helper used by the scheduler to track the last
// fetch time per project so incremental fetching (updatedAfter) works.
type lastUpdatedTimes struct {
	pipelines    map[int]time.Time
	mergeRequests map[int]time.Time
}

func newLastUpdatedTimes() *lastUpdatedTimes {
	return &lastUpdatedTimes{
		pipelines:     make(map[int]time.Time),
		mergeRequests: make(map[int]time.Time),
	}
}
