{{/*
Expand the name of the chart.
*/}}
{{- define "lk-tnsw.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "lk-tnsw.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "lk-tnsw.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
A component's name, <fullname>-<suffix>, called with (list . "<suffix>"). The
fullname is cut to 50 before the suffix is added, so the suffix is never cut
and components never share a name; 50 leaves room for suffixes up to 12
characters (api-migrate is the longest) within the 63-character limit.
*/}}
{{- define "lk-tnsw.componentFullname" -}}
{{- $root := index . 0 -}}
{{- printf "%s-%s" (include "lk-tnsw.fullname" $root | trunc 50 | trimSuffix "-") (index . 1) -}}
{{- end }}

{{/*
Backend component: fullname, selector labels, labels.
*/}}
{{- define "lk-tnsw.backend.fullname" -}}
{{- include "lk-tnsw.componentFullname" (list . "api") -}}
{{- end }}

{{- define "lk-tnsw.backend.selectorLabels" -}}
app.kubernetes.io/name: {{ include "lk-tnsw.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: backend
{{- end }}

{{- define "lk-tnsw.backend.labels" -}}
helm.sh/chart: {{ include "lk-tnsw.chart" . }}
{{ include "lk-tnsw.backend.selectorLabels" . }}
{{- if .Values.backend.image.tag }}
app.kubernetes.io/version: {{ .Values.backend.image.tag | quote }}
{{- else if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Migration component: fullname, selector labels, labels. The name stays
<backend>-migrate, the schema it migrates being the backend's.
*/}}
{{- define "lk-tnsw.migration.fullname" -}}
{{- include "lk-tnsw.componentFullname" (list . "api-migrate") -}}
{{- end }}

{{- define "lk-tnsw.migration.selectorLabels" -}}
app.kubernetes.io/name: {{ include "lk-tnsw.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: migrate
{{- end }}

{{- define "lk-tnsw.migration.labels" -}}
helm.sh/chart: {{ include "lk-tnsw.chart" . }}
{{ include "lk-tnsw.migration.selectorLabels" . }}
{{- with (include "lk-tnsw.migration.imageTag" .) }}
app.kubernetes.io/version: {{ . | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
The migration image's tag before the appVersion fallback: its own, else the
backend's, since release.yml publishes both images from the same git tag.
*/}}
{{- define "lk-tnsw.migration.imageTag" -}}
{{- .Values.migration.image.tag | default .Values.backend.image.tag | default .Chart.AppVersion -}}
{{- end }}

{{/*
Frontend component: fullname, selector labels, labels.
*/}}
{{- define "lk-tnsw.frontend.fullname" -}}
{{- include "lk-tnsw.componentFullname" (list . "web") -}}
{{- end }}

{{- define "lk-tnsw.frontend.selectorLabels" -}}
app.kubernetes.io/name: {{ include "lk-tnsw.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: frontend
{{- end }}

{{- define "lk-tnsw.frontend.labels" -}}
helm.sh/chart: {{ include "lk-tnsw.chart" . }}
{{ include "lk-tnsw.frontend.selectorLabels" . }}
{{- if .Values.frontend.image.tag }}
app.kubernetes.io/version: {{ .Values.frontend.image.tag | quote }}
{{- else if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Image tag: the tag given in values, else the chart's appVersion — so a released
chart deploys the images it was released with. A chart packaged from source
keeps the 0.0.0 placeholder, which no image has, so fail with the fix instead
of an ImagePullBackOff.
Usage: {{ include "lk-tnsw.imageTag" (dict "tag" .Values.backend.image.tag "root" .) }}
*/}}
{{- define "lk-tnsw.imageTag" -}}
{{- $tag := .tag | default .root.Chart.AppVersion | toString -}}
{{- if eq $tag "0.0.0" -}}
{{- fail "no image tag: this chart was not packaged by a release, so its appVersion is the 0.0.0 placeholder. Install a released chart (helm install lk-tnsw oci://ghcr.io/opennsw/charts/lk-tnsw --version X.Y.Z), or set backend.image.tag and frontend.image.tag." -}}
{{- end -}}
{{- $tag -}}
{{- end -}}

{{/*
The server's config.yaml, rendered from backend.config for the backend
ConfigMap.
*/}}
{{- define "lk-tnsw.backend.configYAML" -}}
{{ toYaml (.Values.backend.config | default dict) }}
{{- end }}
