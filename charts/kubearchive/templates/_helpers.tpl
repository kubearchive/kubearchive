{{/*
Copyright KubeArchive Authors
SPDX-License-Identifier: Apache-2.0
*/}}


{{/* Create environment variables for OpenTelemetry */}}
{{- define "kubearchive.v1.otel.env" -}}
{{- $enabled := .Values.integrations.observability.enabled -}}
{{- $endpoint := .Values.integrations.observability.endpoint -}}

{{/* If 'enabled' we deploy a traces collector, so override the endpoint to use it */}}
{{- if (eq $enabled true) -}}
    {{- $endpoint = tpl "http://otel-collector.{{ .Release.Namespace }}.svc.cluster.local:4318" . -}}
{{- end -}}

- name: KUBEARCHIVE_OTEL_ENABLED
  value: '{{ ternary "false" "true" (eq $endpoint "") }}'
- name: OTEL_EXPORTER_OTLP_ENDPOINT
  value: "{{ $endpoint }}"
{{- end -}}
