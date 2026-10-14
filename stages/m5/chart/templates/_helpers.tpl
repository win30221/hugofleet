{{- define "hf.labels" -}}
app.kubernetes.io/managed-by: Helm
app.kubernetes.io/instance: {{ .Release.Name }}
platform.hugofleet.io/cluster: {{ .Values.cluster | quote }}
{{- end -}}
{{- define "hf.annotations" -}}
meta.helm.sh/release-name: {{ .Release.Name | quote }}
meta.helm.sh/release-namespace: {{ .Release.Namespace | quote }}
{{- end -}}
{{- define "hf.security" -}}
allowPrivilegeEscalation: false
readOnlyRootFilesystem: true
capabilities:
  drop: [ALL]
{{- end -}}
{{- define "hf.podSecurity" -}}
runAsNonRoot: true
runAsUser: 65532
runAsGroup: 65532
seccompProfile:
  type: RuntimeDefault
{{- end -}}
