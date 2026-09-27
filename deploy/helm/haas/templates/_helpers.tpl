{{- define "haas.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "haas.selector" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "haas.labels" -}}
{{ include "haas.selector" . }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
An image by digest, and nothing else. Called with (list "server" .Values.image.server).
*/}}
{{- define "haas.image" -}}
{{- $name := index . 0 -}}
{{- $image := index . 1 -}}
{{- if not $image.repository -}}
{{- fail (printf "image.%s.repository is empty: build and push the image, then name it (README.md, \"Images\")" $name) -}}
{{- end -}}
{{- if not (regexMatch "^sha256:[a-f0-9]{64}$" (default "" $image.digest)) -}}
{{- fail (printf "image.%s.digest must be sha256:<64 hex>, and a tag is not accepted in its place" $name) -}}
{{- end -}}
{{- printf "%s@%s" $image.repository $image.digest -}}
{{- end -}}

{{- define "haas.secretName" -}}
{{- default (include "haas.fullname" .) .Values.auth.existingSecret -}}
{{- end -}}

{{- define "haas.modelSecretName" -}}
{{- default (include "haas.fullname" .) .Values.model.existingSecret -}}
{{- end -}}

{{- define "haas.claimName" -}}
{{- default (printf "%s-data" (include "haas.fullname" .)) .Values.persistence.existingClaim -}}
{{- end -}}
