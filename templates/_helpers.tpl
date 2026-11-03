{{/*
Parses an image tag of the form "{pg}-spock{spock}-..." (e.g.
"18-spock6-standard" or "18.3-spock5.0.6-standard") into its Postgres and
Spock major versions. Returns "pgMajor: 0\nspockMajor: 0" (parse as YAML
with fromYaml) if imageName doesn't match that convention — a digest
reference, a custom tag, or a registry path containing other digits — so
callers must treat 0 as "unknown", not as a real major version.
*/}}
{{- define "pgedge.imageTagVersions" -}}
{{- $tagOnly := last (splitList "/" .) -}}
{{- if contains ":" $tagOnly }}
  {{- $tagOnly = last (splitList ":" $tagOnly) }}
{{- end }}
{{- $extracted := regexReplaceAll "^([0-9]+)(?:\\.[0-9]+){0,2}-spock([0-9]+)(?:\\.[0-9]+){0,2}[-_].*$" $tagOnly "${1}|${2}" }}
{{- if ne $extracted $tagOnly }}
pgMajor: {{ (splitList "|" $extracted) | first }}
spockMajor: {{ (splitList "|" $extracted) | last }}
{{- else }}
pgMajor: 0
spockMajor: 0
{{- end }}
{{- end }}

{{- define "validate.newNodes" -}}
{{- if and .Release.IsUpgrade .Values.pgEdge.initSpock }}
  {{- $cfgName := printf "%s-config" .Values.pgEdge.appName }}
  {{- $oldCfg := lookup "v1" "ConfigMap" .Release.Namespace $cfgName }}
  {{- $oldNames := dict }}
  {{- if $oldCfg }}
    {{- $parsed := fromYamlArray ($oldCfg.data.nodes | default "") }}
    {{- range $parsed }}
      {{- $oldName := default "" (get . "name") }}
      {{- if ne $oldName "" }}
        {{- $_ := set $oldNames $oldName true }}
      {{- end }}
    {{- end }}
  {{- end }}

  {{- $all := concat (default (list) .Values.pgEdge.nodes) (default (list) .Values.pgEdge.externalNodes) }}
  {{- range $n := $all }}
    {{- $name := default "" (get $n "name") }}
    {{- $bootstrap := (get $n "bootstrap" | default dict) }}
    {{- $mode := default "" (get $bootstrap "mode") }}

    {{- if not (hasKey $oldNames $name) }}
      {{- if eq $mode "" }}
        {{ fail (printf "New node %s must specify bootstrap.mode" $name) }}
      {{- end }}
    {{- else }}
      {{- if ne $mode "" }}
        {{ fail (printf "Existing node %s cannot be re-bootstrapped (bootstrap.mode=%s)" $name $mode) }}
      {{- end }}
    {{- end }}
  {{- end }}
{{- end }}
{{- end }}
