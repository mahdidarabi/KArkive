{{- define "karkive.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "karkive.fullname" -}}
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

{{- define "karkive.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | quote }}
app.kubernetes.io/name: {{ include "karkive.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: karkive
{{- end }}

{{- define "karkive.selectorLabels" -}}
app.kubernetes.io/name: {{ include "karkive.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "karkive.operatorName" -}}
{{- printf "%s-operator" (include "karkive.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "karkive.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "karkive.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "karkive.webhookServiceName" -}}
{{- printf "%s-webhook" (include "karkive.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "karkive.webhookSecretName" -}}
{{- printf "%s-tls" (include "karkive.webhookServiceName" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "karkive.webhookCertificateName" -}}
{{- printf "%s-webhook" (include "karkive.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "karkive.webhookIssuerName" -}}
{{- printf "%s-webhook-selfsigned" (include "karkive.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}
