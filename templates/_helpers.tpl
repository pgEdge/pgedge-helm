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
