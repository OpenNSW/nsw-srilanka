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
Backend component: fullname, selector labels, labels.
*/}}
{{- define "lk-tnsw.backend.fullname" -}}
{{- printf "%s-api" (include "lk-tnsw.fullname" .) | trunc 63 | trimSuffix "-" -}}
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
Frontend component: fullname, selector labels, labels.
*/}}
{{- define "lk-tnsw.frontend.fullname" -}}
{{- printf "%s-web" (include "lk-tnsw.fullname" .) | trunc 63 | trimSuffix "-" -}}
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
