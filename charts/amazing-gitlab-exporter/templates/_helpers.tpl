{{/*
Expand the name of the chart.
*/}}
{{- define "amazing-gitlab-exporter.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "amazing-gitlab-exporter.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "amazing-gitlab-exporter.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "amazing-gitlab-exporter.labels" -}}
helm.sh/chart: {{ include "amazing-gitlab-exporter.chart" . }}
{{ include "amazing-gitlab-exporter.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "amazing-gitlab-exporter.selectorLabels" -}}
app.kubernetes.io/name: {{ include "amazing-gitlab-exporter.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "amazing-gitlab-exporter.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "amazing-gitlab-exporter.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Return the full image reference (supports digest or tag).
*/}}
{{- define "amazing-gitlab-exporter.image" -}}
{{- if .Values.image.digest }}
{{- printf "%s@%s" .Values.image.repository .Values.image.digest }}
{{- else }}
{{- printf "%s:%s" .Values.image.repository (default .Chart.AppVersion .Values.image.tag) }}
{{- end }}
{{- end }}

{{/*
Compute the Redis URL.
Priority: sub-chart > external.
*/}}
{{- define "amazing-gitlab-exporter.redisUrl" -}}
{{- if .Values.redis.enabled }}
{{- if .Values.redis.auth.enabled }}
{{- printf "redis://:%s@%s-redis-master:6379/0" (default "" .Values.redis.auth.password) .Release.Name }}
{{- else }}
{{- printf "redis://%s-redis-master:6379/0" .Release.Name }}
{{- end }}
{{- else if .Values.externalRedis.url }}
{{- .Values.externalRedis.url }}
{{- end }}
{{- end }}

{{/*
Compute the Prometheus server URL (for Grafana datasource).
Priority: co-deployed sub-chart > external.
*/}}
{{- define "amazing-gitlab-exporter.prometheusUrl" -}}
{{- if .Values.prometheus.enabled }}
{{- printf "http://%s-prometheus-server:80" .Release.Name }}
{{- else if .Values.externalPrometheus.url }}
{{- .Values.externalPrometheus.url }}
{{- end }}
{{- end }}

{{/*
Detect OpenShift by checking for route.openshift.io API.
*/}}
{{- define "amazing-gitlab-exporter.isOpenShift" -}}
{{- if .Capabilities.APIVersions.Has "route.openshift.io/v1" -}}true{{- end -}}
{{- end }}

{{/*
Return the name of the Secret to use.
*/}}
{{- define "amazing-gitlab-exporter.secretName" -}}
{{- if .Values.existingSecret.name }}
{{- .Values.existingSecret.name }}
{{- else }}
{{- include "amazing-gitlab-exporter.fullname" . }}
{{- end }}
{{- end }}

{{/*
Should dashboards be enabled?
Explicit grafanaDashboards.enabled OR implicitly when grafana sub-chart is enabled.
*/}}
{{- define "amazing-gitlab-exporter.dashboardsEnabled" -}}
{{- if or .Values.grafanaDashboards.enabled .Values.grafana.enabled -}}true{{- end -}}
{{- end }}
