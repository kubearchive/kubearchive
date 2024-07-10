{{/*
Copyright KubeArchive Authors
SPDX-License-Identifier: Apache-2.0
*/}}

{{/*
Create environment variables for OpenTelemetry if .observability is set to true. Otherwise set KUBEARCHIVE_OTEL_ENABLED=false.
This tells the OpenTelemtry instrumentation if it should start or not.
*/}}
{{- define "kubearchive.v1.otel.env" -}}
{{- if .observability -}}
- name: KUBEARCHIVE_OTEL_ENABLED
  value: "true"
{{- else -}}
- name: KUBEARCHIVE_OTEL_ENABLED
  value: "false"
{{- end -}}
{{- end -}}

