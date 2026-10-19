{{- define "gusto.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "gusto.selector" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "gusto.labels" -}}
{{ include "gusto.selector" . }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
An image by digest, and nothing else. Called with (list "server" .Values.image.server).
*/}}
{{- define "gusto.image" -}}
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

{{- define "gusto.secretName" -}}
{{- default (include "gusto.fullname" .) .Values.auth.existingSecret -}}
{{- end -}}

{{- define "gusto.modelSecretName" -}}
{{- default (include "gusto.fullname" .) .Values.model.existingSecret -}}
{{- end -}}

{{- define "gusto.oidcSecretName" -}}
{{- default (include "gusto.fullname" .) .Values.auth.oidc.existingSecret -}}
{{- end -}}

{{- define "gusto.claimName" -}}
{{- default (printf "%s-data" (include "gusto.fullname" .)) .Values.persistence.existingClaim -}}
{{- end -}}
