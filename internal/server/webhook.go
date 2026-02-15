package server

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/sirupsen/logrus"
)

// WebhookHandler handles incoming GitLab webhook events and dispatches them
// to registered callbacks. It validates the X-Gitlab-Token header when a
// secret token is configured.
type WebhookHandler struct {
	secretToken     string
	logger          *logrus.Entry
	onPipelineEvent func(projectPath string)
	onMREvent       func(projectPath string)
}

// NewWebhookHandler creates a handler that optionally validates webhooks using
// secretToken. If secretToken is empty, token validation is skipped.
func NewWebhookHandler(secretToken string, logger *logrus.Entry) *WebhookHandler {
	return &WebhookHandler{
		secretToken: secretToken,
		logger:      logger.WithField("component", "webhook"),
	}
}

// SetOnPipelineEvent registers a callback invoked for pipeline events.
func (wh *WebhookHandler) SetOnPipelineEvent(fn func(projectPath string)) {
	wh.onPipelineEvent = fn
}

// SetOnMREvent registers a callback invoked for merge request events.
func (wh *WebhookHandler) SetOnMREvent(fn func(projectPath string)) {
	wh.onMREvent = fn
}

// webhookPayload is the minimal envelope used to determine the event type and
// extract the project path.
type webhookPayload struct {
	ObjectKind string `json:"object_kind"`
	Project    struct {
		PathWithNamespace string `json:"path_with_namespace"`
	} `json:"project"`
}

// ServeHTTP implements http.Handler. It validates the token (if configured),
// reads the JSON body, and dispatches to the appropriate callback.
func (wh *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Validate secret token if configured.
	if wh.secretToken != "" {
		token := r.Header.Get("X-Gitlab-Token")
		if token != wh.secretToken {
			wh.logger.Warn("webhook received with invalid token")
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}

	// Read body (limit to 1 MB to prevent abuse).
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		wh.logger.WithError(err).Error("failed to read webhook body")
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		wh.logger.WithError(err).Error("failed to parse webhook payload")
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	projectPath := payload.Project.PathWithNamespace
	if projectPath == "" {
		wh.logger.Warn("webhook payload missing project path")
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	wh.logger.WithFields(logrus.Fields{
		"object_kind": payload.ObjectKind,
		"project":     projectPath,
	}).Debug("webhook event received")

	switch payload.ObjectKind {
	case "pipeline":
		if wh.onPipelineEvent != nil {
			wh.onPipelineEvent(projectPath)
		}
	case "merge_request":
		if wh.onMREvent != nil {
			wh.onMREvent(projectPath)
		}
	default:
		wh.logger.WithField("object_kind", payload.ObjectKind).
			Debug("ignoring unhandled webhook event type")
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"accepted"}`))
}
