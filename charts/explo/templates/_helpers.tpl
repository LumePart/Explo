{{/*
Expand the name of the chart.
*/}}
{{- define "explo.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "explo.fullname" -}}
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
Common labels
*/}}
{{- define "explo.labels" -}}
helm.sh/chart: {{ include "explo.name" . }}-{{ .Chart.Version | replace "+" "_" }}
{{ include "explo.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "explo.selectorLabels" -}}
app.kubernetes.io/name: {{ include "explo.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Image tag defaults to Chart appVersion.
*/}}
{{- define "explo.imageTag" -}}
{{- .Values.image.tag | default .Chart.AppVersion }}
{{- end }}

{{/*
Auth secret name when using inline credentials.
*/}}
{{- define "explo.authSecretName" -}}
{{- if .Values.auth.existingSecret }}
{{- .Values.auth.existingSecret }}
{{- else }}
{{- include "explo.fullname" . }}-auth
{{- end }}
{{- end }}

{{/*
True when UI credentials are injected at install time.
*/}}
{{- define "explo.authConfigured" -}}
{{- if .Values.auth.existingSecret -}}
true
{{- else if and .Values.auth.username .Values.auth.password -}}
true
{{- else -}}
false
{{- end }}
{{- end }}

{{/*
Config PVC claim name.
*/}}
{{- define "explo.configClaimName" -}}
{{- if .Values.persistence.config.existingClaim }}
{{- .Values.persistence.config.existingClaim }}
{{- else }}
{{- include "explo.fullname" . }}-config
{{- end }}
{{- end }}

{{/*
Media PVC claim name.
*/}}
{{- define "explo.mediaClaimName" -}}
{{- if .Values.persistence.media.existingClaim }}
{{- .Values.persistence.media.existingClaim }}
{{- else }}
{{- include "explo.fullname" . }}-media
{{- end }}
{{- end }}
