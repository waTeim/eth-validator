{{/*
Expand the name of the chart.
*/}}
{{- define "eth-validator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "eth-validator.fullname" -}}
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
{{- define "eth-validator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}


{{/*
Common labels
*/}}
{{- define "eth-validator.labels" -}}
helm.sh/chart: {{ include "eth-validator.chart" . }}
{{ include "eth-validator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Geth common labels
*/}}
{{- define "eth-validator-geth.labels" -}}
helm.sh/chart: {{ include "eth-validator.chart" . }}
{{ include "eth-validator-geth.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Lighthouse beacon common labels
*/}}
{{- define "eth-validator-lighthouse-beacon.labels" -}}
helm.sh/chart: {{ include "eth-validator.chart" . }}
{{ include "eth-validator-lighthouse-beacon.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Lighthouse validator common labels
*/}}
{{- define "eth-validator-lighthouse-validator.labels" -}}
helm.sh/chart: {{ include "eth-validator.chart" . }}
{{ include "eth-validator-lighthouse-validator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "eth-validator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "eth-validator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Geth selector labels
*/}}
{{- define "eth-validator-geth.selectorLabels" -}}
app.kubernetes.io/name: {{ include "eth-validator.name" . }}-geth
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Lighthouse beacon selector labels
*/}}
{{- define "eth-validator-lighthouse-beacon.selectorLabels" -}}
app.kubernetes.io/name: {{ include "eth-validator.name" . }}-lighthouse-beacon
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Lighthouse validator selector labels
*/}}
{{- define "eth-validator-lighthouse-validator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "eth-validator.name" . }}-lighthouse-validator
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "eth-validator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "eth-validator.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Execution Endpoint
*/}}
{{- define "eth-validator.executionEndpoint" -}}
{{- printf "http://%s-geth:%d" (include "eth-validator.fullname" .) (int .Values.geth.internal.auth.port) }}
{{- end }}

{{/*
Beacon Nodes
*/}}
{{- define "eth-validator.beaconNodes" -}}
{{- printf "http://%s-lighthouse-beacon:%d" (include "eth-validator.fullname" .) (int .Values.lighthouseBeacon.internal.api.port) }}
{{- end }}

{{/*
Beacon Pod
*/}}
{{- define "eth-validator.beaconPod" -}}
{{- printf "%s-lighthouse-beacon-0" (include "eth-validator.fullname" .) }}
{{- end }}