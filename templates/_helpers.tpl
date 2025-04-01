{{- define "pgedge.v0.svcName" -}}
{{- printf "%s" $.Values.service.name -}}
{{- end -}}

{{- define "pgedge.v0.headlessSvcName" -}}
{{- printf "%s-hl" $.Values.service.name -}}
{{- end -}}

{{- define "pgedge.v0.dbJsonName" -}}
{{- printf "%s-dbjson" $.Values.service.name -}}
{{- end -}}

{{- define "pgedge.v0.usersSecret" -}}
{{- if $.Values.pgEdge.existingUsersSecret }}
{{- $.Values.pgEdge.existingUsersSecret -}}
{{- else }}
{{- printf "%s-users" $.Values.service.name -}}
{{- end }}
{{- end -}}

{{- define "pgedge.v0.dbSpec.users" -}}
{{- $pgedgeUser := dict
  "password" (randAlphaNum 24)
  "username" "pgedge"
  "superuser" true
  "service" "postgres"
  "type" "internal_admin"
}}
{{- $users := list $pgedgeUser }}
{{- range $i, $u := $.Values.pgEdge.dbSpec.users }}
{{- $pw := dict "password" (randAlphaNum 24) }}
{{- $user := merge $u $pw }}
{{- $users = append $users $user }}
{{- end }}
{{- dict "users" $users | toJson -}}
{{- end -}}

{{- define "pgedge.v0.dbSpec.nodes" -}}
{{- if $.Values.pgEdge.dbSpec.nodes }}
{{- $.Values.pgEdge.dbSpec.nodes | toJson -}}
{{- else }}
{{- $nodes := list }}
{{- range $i, $_ := until ($.Values.pgEdge.nodeCount | int) }}
{{- $nodeName := printf "%s-%d" $.Values.pgEdge.appName $i }}
{{- $svcName := include "pgedge.v0.headlessSvcName" $ }}
{{- $hostname := printf "%s.%s.%s.svc.%s" $nodeName $svcName $.Release.Namespace $.Values.global.clusterDomain }}
{{- $node := dict "name" $nodeName "hostname" $hostname }}
{{- $nodes = append $nodes $node }}
{{- end }}
{{- $nodes | toJson -}}
{{- end }}
{{- end -}}

{{- define "pgedge.v0.dbSpec" -}}
{{- $dbSpec := dict
  "name" $.Values.pgEdge.dbSpec.dbName
  "port" $.Values.pgEdge.port
  "nodes" (include "pgedge.v0.dbSpec.nodes" $ | fromJsonArray)
  "options" $.Values.pgEdge.dbSpec.options
}}
{{- $dbSpec | toJson -}}
{{- end -}}

{{- define "pgedge.v0.probeExec" -}}
{{- $cmd := list
  "/bin/sh"
  "-c"
  (printf "pg_isready -U pgedge -d %s" $.Values.pgEdge.dbSpec.dbName)
}}
{{- dict "command" $cmd | toYaml -}}
{{- end -}}

{{- define "pgedge.v0.readinessProbe" -}}
{{- $exec := include "pgedge.v0.probeExec" . | fromYaml }}
{{- $probe := omit $.Values.pgEdge.readinessProbe "enabled" }}
{{- merge $probe (dict "exec" $exec) | toYaml -}}
{{- end -}}

{{- define "pgedge.v0.livenessProbe" -}}
{{- $exec := include "pgedge.v0.probeExec" . | fromYaml }}
{{- $probe := omit $.Values.pgEdge.readinessProbe "enabled" }}
{{- merge $probe (dict "exec" $exec) | toYaml -}}
{{- end -}}

{{- define "pgedge.v0.labels" -}}
app.kubernetes.io/name: {{ $.Values.pgEdge.appName }}
app.kubernetes.io/component: database
app.kubernetes.io/managed-by: helm
{{- if $.Values.labels }}
{{ $.Values.labels | toYaml }}
{{- end }}
{{- end -}}

{{- define "pgedge.v0.matchLabels" -}}
app.kubernetes.io/name: {{ $.Values.pgEdge.appName }}
{{- if $.Values.pgEdge.extraMatchLabels }}
{{ $.Values.pgEdge.extraMatchLabels | toYaml}}
{{- end }}
{{- end -}}

{{- define "pgedge.v0.affinity" -}}
{{- if $.Values.pgEdge.podAffinity -}}
podAffinity: {{- $.Values.pgEdge.podAffinity | toYaml | nindent 2 }}
{{- end }}
{{- if $.Values.pgEdge.podAntiAffinityEnabled -}}
{{- if $.Values.pgEdge.podAntiAffinityOverride }}
podAntiAffinity: {{- $.Values.pgEdge.podAntiAffinityOverride | toYaml | nindent 2 }}
{{- else -}}
podAntiAffinity:
  preferredDuringSchedulingIgnoredDuringExecution:
    - podAffinityTerm:
        labelSelector:
          matchLabels: {{- include "pgedge.v0.matchLabels" . | nindent 12 }}
        topologyKey: kubernetes.io/hostname
      weight: 1
{{- end -}}
{{- end -}}
{{- if $.Values.pgEdge.nodeAffinity }}
nodeAffinity: {{- $.Values.pgEdge.nodeAffinity | toYaml | nindent 2 }}
{{- end }}
{{- end -}}
