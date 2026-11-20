{{- define "lg.labels" -}}
app.kubernetes.io/part-of: llm-gateway
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end }}

{{/* Waits until host:port accepts TCP. The gateway and billing exit when MySQL is not up yet. */}}
{{- define "lg.wait" -}}
- name: wait-{{ .name }}
  image: {{ .root.Values.images.busybox }}
  imagePullPolicy: {{ .root.Values.pullPolicy }}
  command: ["sh", "-c", "until nc -z {{ .host }} {{ .port }}; do echo waiting for {{ .host }}:{{ .port }}; sleep 2; done"]
  securityContext: { allowPrivilegeEscalation: false, runAsNonRoot: true, runAsUser: 65534, capabilities: { drop: [ALL] } }
{{- end }}

{{- define "lg.appsec" -}}
securityContext:
  runAsNonRoot: true
  runAsUser: {{ .uid }}
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities: { drop: [ALL] }
{{- end }}
