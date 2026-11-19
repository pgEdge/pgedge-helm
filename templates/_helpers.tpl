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

{{- /*
pgedge.nativeSlotSync turns on CloudNativePG's logical decoding slot
synchronization for a node. It takes a dict with "spec" (the node's merged
clusterSpec, changed in place) and "mode" (pgEdge.nativeSlotSync):

  auto   on when the image tag (<pg>[.<minor>]-spock<major>[...]) names
         PostgreSQL 18+ and Spock 6+, where Spock no longer runs its own
         failover slot worker. Off for anything else, including tags it
         can't parse (digests, imageCatalogRef, custom tags).
  true   always on, e.g. for images the tag check can't parse, or to use
         native slot sync on PostgreSQL 17.
  false  never on.

Turning it on sets:

  replicationSlots.highAvailability.synchronizeLogicalDecoding: true
  postgresql.parameters.sync_replication_slots: "on"
  postgresql.parameters.hot_standby_feedback: "on" (only if unset)

Nothing is changed when the node has a single instance (there is no
standby to copy slots to, and CNPG can list the primary's own HA slot in
synchronized_standby_slots, which holds back all logical decoding on the
node), when CNPG's HA replication slots are disabled (there is no physical
slot for native slot sync to use), or when the user set
synchronizeLogicalDecoding or sync_replication_slots themselves.
*/ -}}
{{- define "pgedge.nativeSlotSync" -}}
{{- $spec := .spec -}}
{{- $mode := .mode -}}
{{- $enable := false -}}
{{- if eq $mode "true" -}}
  {{- $enable = true -}}
{{- else if eq $mode "auto" -}}
  {{- $tag := trimPrefix ":" (regexFind ":[^:/]+$" (toString ($spec.imageName | default ""))) -}}
  {{- if regexMatch "^[0-9]+(\\.[0-9]+)*-spock[0-9]+([.-]|$)" $tag -}}
    {{- $pgMajor := regexReplaceAll "^([0-9]+).*$" $tag "${1}" | atoi -}}
    {{- $spockMajor := regexReplaceAll "^[0-9.]+-spock([0-9]+).*$" $tag "${1}" | atoi -}}
    {{- if and (ge $pgMajor 18) (ge $spockMajor 6) -}}
      {{- $enable = true -}}
    {{- end -}}
  {{- end -}}
{{- end -}}
{{- if le (int (default 1 $spec.instances)) 1 -}}
  {{- $enable = false -}}
{{- end -}}
{{- if $enable -}}
  {{- $slots := $spec.replicationSlots | default dict -}}
  {{- $ha := $slots.highAvailability | default dict -}}
  {{- $haEnabled := true -}}
  {{- if and (hasKey $ha "enabled") (not (kindIs "invalid" $ha.enabled)) -}}
    {{- $haEnabled = $ha.enabled -}}
  {{- end -}}
  {{- $params := $spec.postgresql.parameters -}}
  {{- if and $haEnabled (not (hasKey $ha "synchronizeLogicalDecoding")) (not (hasKey $params "sync_replication_slots")) -}}
    {{- $_ := set $ha "synchronizeLogicalDecoding" true -}}
    {{- $_ := set $slots "highAvailability" $ha -}}
    {{- $_ := set $spec "replicationSlots" $slots -}}
    {{- $_ := set $params "sync_replication_slots" "on" -}}
    {{- if not (hasKey $params "hot_standby_feedback") -}}
      {{- $_ := set $params "hot_standby_feedback" "on" -}}
    {{- end -}}
  {{- end -}}
{{- end -}}
{{- end -}}
