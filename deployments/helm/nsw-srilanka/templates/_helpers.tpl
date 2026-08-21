{{/*
Expand the name of the chart.
*/}}
{{- define "nsw-srilanka.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "nsw-srilanka.fullname" -}}
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
{{- define "nsw-srilanka.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Backend component: fullname, selector labels, labels.
*/}}
{{- define "nsw-srilanka.backend.fullname" -}}
{{- printf "%s-api" (include "nsw-srilanka.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{- define "nsw-srilanka.backend.selectorLabels" -}}
app.kubernetes.io/name: {{ include "nsw-srilanka.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: backend
{{- end }}

{{- define "nsw-srilanka.backend.labels" -}}
helm.sh/chart: {{ include "nsw-srilanka.chart" . }}
{{ include "nsw-srilanka.backend.selectorLabels" . }}
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
{{- define "nsw-srilanka.frontend.fullname" -}}
{{- printf "%s-web" (include "nsw-srilanka.fullname" .) | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{- define "nsw-srilanka.frontend.selectorLabels" -}}
app.kubernetes.io/name: {{ include "nsw-srilanka.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: frontend
{{- end }}

{{- define "nsw-srilanka.frontend.labels" -}}
helm.sh/chart: {{ include "nsw-srilanka.chart" . }}
{{ include "nsw-srilanka.frontend.selectorLabels" . }}
{{- if .Values.frontend.image.tag }}
app.kubernetes.io/version: {{ .Values.frontend.image.tag | quote }}
{{- else if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}
