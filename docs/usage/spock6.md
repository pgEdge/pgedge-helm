# Using Spock 6.0

Spock 6.0 is the next major release of Spock, the active-active replication extension in pgEdge Distributed Postgres. This chart supports deploying new clusters on Spock 6.0 and upgrading existing Spock 5.0.x clusters to Spock 6.0.

!!! warning

    Spock 6.0 is in beta. Spock 5 stays the chart's default until Spock 6.0 is generally available, and Spock 6.0 is opt-in until then. Test Spock 6.0 in a separate environment before you use it for data you care about.

The chart and the init-spock job work with both Spock 5.0.x and Spock 6.0; the job detects the versions on each node and adjusts. For the full list of changes in Spock 6.0, see the [Spock 6.0 release notes](https://docs.pgedge.com/spock-v6/v6-0-0-beta/spock_release_notes/).

## Choosing a Spock 6.0 image

You select Spock 6.0 through `clusterSpec.imageName`, using an image tag that names Spock 6. The tags follow the same pattern as the Spock 5 tags:

```yaml
pgEdge:
  clusterSpec:
    imageName: ghcr.io/pgedge/pgedge-postgres:18-spock6-standard
```

The following table shows the mutable Spock 6.0 tags:

| Tag | PostgreSQL | Spock |
|-----|------------|-------|
| `18-spock6-standard` | 18 | 6.0 |
| `17-spock6-standard` | 17 | 6.0 |
| `16-spock6-standard` | 16 | 6.0 |

The `minimal` image flavor is also available, for example `18-spock6-minimal`. For production, pin an immutable tag that names the exact PostgreSQL and Spock versions; see [Upgrading Postgres](postgres_upgrades.md) for how tags work.

!!! note

    Upgrading an existing Spock 5.0.x cluster requires Spock 6.0.0-beta.2 or later, which has the upgrade path from Spock 5.0.14. Spock 6.0.0-beta.1 has no upgrade path from Spock 5.0.11 or later. Both betas report version `6.0.0`, so check for the upgrade path instead: in a pod that runs the image, `SELECT path FROM pg_extension_update_paths('spock') WHERE source = '5.0.14' AND target = '6.0.0';` returns a path only on beta.2 or later.

    The chart does not move a cluster from beta.1 to beta.2, because both betas have the same version. Follow the beta.1 upgrade steps in the [Spock 6.0 release notes](https://docs.pgedge.com/spock-v6/v6-0-0-beta/spock_release_notes/) instead.

Spock 6.0 images still need `spock_output` in `output_plugin_libraries` on PostgreSQL versions with the fix for CVE-2026-6471. The chart sets this parameter by default; see [output_plugin_libraries and older PostgreSQL minor versions](../configuration.md#output_plugin_libraries-and-older-postgresql-minor-versions).

## Standby instances and failover slots

On PostgreSQL 18, Spock 6.0 no longer runs its own failover slot worker. Standbys receive copies of the Spock replication slots only through PostgreSQL's native slot synchronization, which copies only slots that have the `failover` flag.

The `pgEdge.nativeSlotSync` value controls when the chart turns native slot synchronization on. The following table describes its values:

| Value | Behavior |
|-------|----------|
| `auto` (default) | On for nodes whose image tag names PostgreSQL 18 or later and Spock 6 or later, such as `18-spock6-standard` or `18.7-spock6.0.0-standard`. Off for any other image, including images given by digest or through `imageCatalogRef`. |
| `true` | On for every node. Use it for images whose tag the chart can't read, or to use native slot synchronization on PostgreSQL 17. |
| `false` | Never on. |

When it is on for a node, the chart turns native slot synchronization on through CloudNativePG by setting:

- `replicationSlots.highAvailability.synchronizeLogicalDecoding` to `true`, which makes CloudNativePG manage `synchronized_standby_slots` on the primary.
- `postgresql.parameters.sync_replication_slots` to `on`.
- `postgresql.parameters.hot_standby_feedback` to `on`, unless you set it yourself.

The chart leaves these settings alone in the following cases:

- you set `synchronizeLogicalDecoding` or `sync_replication_slots` yourself, in the chart-wide or the node's `clusterSpec`.
- CloudNativePG's high availability replication slots are disabled with `replicationSlots.highAvailability.enabled: false`.

For a node with more than one instance that runs Spock 6.0 on PostgreSQL 18, the init-spock job checks that `sync_replication_slots` is on, and fails with an error if it isn't. Without it, a promoted standby has no Spock slots, and the other nodes stop replicating from the node. The check uses the versions the node actually runs, so it also catches images that `auto` doesn't recognize.

On PostgreSQL 16 and 17, Spock 6.0 keeps using its own failover slot worker, as Spock 5 does, and `auto` does not change anything. To move an existing PostgreSQL 17 node with standbys to native slot synchronization with `pgEdge.nativeSlotSync: true`, follow the switching steps in the [Spock logical slot failover documentation](https://docs.pgedge.com/spock-v6/v6-0-0-beta/logical_slot_failover/); slot copies made by the Spock worker block native slot synchronization until you drop them.

On nodes running Spock 6.0 on PostgreSQL 17 and later, the init-spock job makes sure that every Spock slot carries the `failover` flag. Spock 6.0 creates new slots with the flag, and the job creates the slots it makes for new nodes with the flag too, on any Spock version. For a slot carried over from Spock 5.0.x without the flag, the job does the following:

- an idle slot gets the flag right away.
- for a slot in use, the job pauses the subscription that reads the slot, waits up to a minute for the slot to be released, sets the flag, and resumes the subscription. Pausing a subscription only stops apply for a moment; the slot keeps its position, so no data is lost or applied twice.
- a slot that no node in the chart's configuration reads only gets a warning in the job log.

If the job can't resume a subscription, it logs an error naming the subscription; resume it with `SELECT spock.sub_enable('<subscription>');` on the subscriber node. The job's errors and warnings appear in its log, which you can read with `kubectl logs job/pgedge-init-spock`.

!!! warning

    With `synchronized_standby_slots` in place, the primary holds back changes to other nodes until every standby has received them. While a standby is down, replication from that node to the other nodes waits for it. This keeps the other nodes from receiving changes that a promoted standby would not have.

Before you promote a standby by hand, check that it has a usable copy of every Spock slot, as described in [Replication slot readiness](standby.md#replication-slot-readiness). With native slot synchronization, every copy must also show `synced = t`.

### spock.pg_standby_slot_names

On PostgreSQL 18, Spock 6.0.0-beta.2 and later apply `spock.pg_standby_slot_names`. With the default `spock.standby_slots_min_confirmed` of `-1`, every logical walsender waits for all named physical slots, so a name without a matching slot stalls replication from that node.

The chart never sets `spock.pg_standby_slot_names`, and CloudNativePG already holds back logical replication through `synchronized_standby_slots`, so you don't need it. If you set it yourself, the init-spock job logs a warning for each node where a named slot doesn't exist, listing the missing slots.

## Upgrading from Spock 5.0.x

You upgrade a cluster from Spock 5.0.x to Spock 6.0 by changing the image, in the same way as a Spock minor version upgrade. The upgrade keeps the PostgreSQL major version; to change both, upgrade one at a time.

Use the following steps to upgrade:

1. Check the Spock version on each node with `SELECT extversion FROM pg_extension WHERE extname = 'spock';`. Spock 6.0 upgrades any 5.0.x release in one step, through 5.0.14.

2. Check your runbooks and automation for direct DDL against the `spock` or `snowflake` schemas. Spock 6.0 rejects such statements unless the session sets `spock.enable_ddl_replication = off` first.

3. Take a backup of each node with CloudNativePG, so you can restore if something goes wrong. Spock can't be downgraded from 6.0 to 5.0.x in place.

4. Change `clusterSpec.imageName` to a Spock 6.0 image for the same PostgreSQL major version, and run `helm upgrade`:

    ```yaml
    pgEdge:
      clusterSpec:
        imageName: ghcr.io/pgedge/pgedge-postgres:18-spock6-standard
    ```

5. Wait for CloudNativePG to roll out the new image on every node; `kubectl get clusters` shows each one as healthy when it's done.

6. Check that the init-spock job completed, with `kubectl get job pgedge-init-spock`.

7. On PostgreSQL 18, if a node has standby instances, drop the slot copies that Spock 5's failover slot worker made on each standby. Native slot synchronization doesn't replace a slot with the same name, and keeps failing on the standby until those copies are gone. Run the following statement on every standby pod, for example with `kubectl exec <pod> -c postgres -- psql -U admin -d app -c "..."`:

    ```sql
    SELECT pg_drop_replication_slot(slot_name)
    FROM pg_replication_slots
    WHERE plugin = 'spock_output' AND NOT synced AND NOT active;
    ```

    Native slot synchronization then creates new copies. Don't promote a standby until its copies show `synced = t` and `temporary = f`, as described in [Replication slot readiness](standby.md#replication-slot-readiness).

During the rollout, Spock's manager worker updates the Spock extension on each primary with `ALTER EXTENSION spock UPDATE` as soon as the primary starts on the new image. The init-spock job waits until the extension on every node matches the Spock binaries before it changes anything, and runs the update itself if it hasn't happened within a minute. The job fails with an error if a node's extension is newer than its binaries, which happens if you roll the image back after the upgrade.

Spock negotiates the replication protocol between nodes, so nodes on Spock 5.0.x and Spock 6.0 keep replicating to each other while the image rolls out.

After the upgrade, check the Spock version on each node with the following query:

```sql
SELECT extversion FROM pg_extension WHERE extname = 'spock';
```

On PostgreSQL 17 and 18, also check that every Spock slot on each primary has the `failover` flag with the following query:

```sql
SELECT slot_name, failover
FROM pg_replication_slots
WHERE plugin = 'spock_output';
```

## Known differences from Spock 5

Spock 6.0 changes some catalog objects and settings that you might use directly. The differences that matter when you use this chart are:

- `spock.progress` is now a view over `spock.apply_group_progress()` and has no `remote_lsn` column; use `remote_commit_lsn` instead.
- the `spock.lag_tracker` view replaces its `last_received_lsn` column with `commit_lsn` and `received_lsn`. The `replication_lag` column is now the time between the last applied commit on the origin and its apply, so it no longer grows while replication is idle or stalled; check alerts built on this view.
- `spock.restart_delay_default`, `spock.restart_delay_on_exception`, and `spock.output_delay` accept values with units, such as `'5s'`, and the chart passes such values through unchanged.
- `spock.wait_for_apply_worker()` is removed, and `spock.use_spi`, `spock.feedback_frequency`, and `spock.batch_inserts` no longer have any effect; remove them from `postgresql.parameters`.
- the apply worker now restarts and retries after a temporary error, such as a deadlock, every `spock.restart_delay_default` instead of disabling the subscription.

Spock 6.0 also adds new settings. The chart doesn't set them, so each keeps its Spock default unless you set it in `postgresql.parameters`. The following table describes the new settings you are most likely to change:

| Setting | Default | Restart required | Description |
|---------|---------|------------------|-------------|
| `spock.sync_timeout` | `0` | No | The time limit for one wait in Spock's node management routines. `0` keeps each routine's built-in limit. |
| `spock.event_history_size` | `1024` | Yes | The number of recent events the `spock.events` view keeps. |
| `spock.non_conflicting_inserts` | `off` | Yes | Asserts that a replicated `INSERT` never collides with a local row, so the apply worker skips the conflict check. Only turn it on if every node generates keys from its own range. |

## Monitoring Spock 6.0

Spock 6.0 adds monitoring views, such as `spock.node_status`, `spock.subscription_status`, `spock.slot_status`, and `spock.events`. Members of the `pg_monitor` role can read them, but a monitoring role also needs `USAGE` on the `spock` schema.

You can create a monitoring role with the chart's managed roles, then grant it access to the `spock` schema. The following values create a `monitor` role:

```yaml
pgEdge:
  clusterSpec:
    managed:
      roles:
        - name: admin
          ensure: present
          comment: Admin role
          login: true
          superuser: true
        - name: monitor
          ensure: present
          login: true
          inRoles:
            - pg_monitor
```

The `managed.roles` list replaces the chart's default list, so the example keeps the `admin` role. To connect as the role from outside the pod, also add a `pg_hba` entry and a client certificate for it, as described in [Creating additional users](../configuration.md#creating-additional-users). Next, run the following statement on each node to grant the role access to the `spock` schema:

```sql
GRANT USAGE ON SCHEMA spock TO monitor;
```

For a support case, `spock.node_report()` returns the whole state of a node as text, including versions, settings, every monitoring view, and recent errors and events. The following command saves the report from a node to a file:

```shell
kubectl exec pgedge-n1-1 -c postgres -- psql -U admin -d app -Atc "SELECT * FROM spock.node_report();" > n1-report.txt
```

For a description of each view, see the [Spock monitoring documentation](https://docs.pgedge.com/spock-v6/v6-0-0-beta/monitoring/).

## Next Steps

The following pages cover related tasks:

- The [Configuring Standby Instances](standby.md) document describes how to add standbys and promote them.
- The [Upgrading Postgres](postgres_upgrades.md) document explains how to upgrade PostgreSQL and Spock minor and major versions.
- The [Configuration](../configuration.md) document describes the chart's Spock and PostgreSQL settings.
