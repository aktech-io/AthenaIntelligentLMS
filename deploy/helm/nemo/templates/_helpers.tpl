{{/* Image reference for a Go service */}}
{{- define "nemo.serviceImage" -}}
{{- $root := index . 0 -}}{{- $svc := index . 1 -}}
{{- if $svc.image -}}
{{ $svc.image }}
{{- else if $root.Values.global.imageRegistry -}}
{{ $root.Values.global.imageRegistry }}/nemo-{{ $svc.name }}:{{ $root.Values.global.imageTag }}
{{- else -}}
nemo-{{ $svc.name }}:{{ $root.Values.global.imageTag }}
{{- end -}}
{{- end -}}

{{/* Effective infra hostnames */}}
{{- define "nemo.dbHost" -}}
{{- if .Values.db.host }}{{ .Values.db.host }}{{ else }}nemo-postgres{{ end -}}
{{- end -}}
{{- define "nemo.rabbitHost" -}}
{{- if .Values.rabbitmq.host }}{{ .Values.rabbitmq.host }}{{ else }}nemo-rabbitmq{{ end -}}
{{- end -}}

{{/* Secret name providing platform credentials */}}
{{- define "nemo.secretName" -}}
{{- if .Values.global.existingSecret }}{{ .Values.global.existingSecret }}{{ else }}nemo-credentials{{ end -}}
{{- end -}}

{{- define "nemo.labels" -}}
app.kubernetes.io/part-of: nemo
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
