# Releasing

This document describes the release process for pgEdge Helm.

## Prerequisites

- [changie](https://changie.dev/guide/installation/) must be installed
- Write access to the repository
- Ability to push tags

## Adding Changelog Entries

As you work on features and fixes, create changelog entries using changie:

```shell
changie new
```

This will prompt you to:
1. Select the type of change (Added, Changed, Fixed, etc.)
2. Enter a description of the change

The entry is saved to `changes/unreleased/` and will be included in the next release.

### Change Types

- **Added** - New features
- **Changed** - Changes to existing functionality
- **Deprecated** - Features that will be removed in future versions
- **Removed** - Features that have been removed
- **Fixed** - Bug fixes
- **Security** - Security-related changes

## Creating a Release

### 1. Ensure Changelog Entries Exist

Before releasing, verify that `changes/unreleased/` contains entries for all notable changes since the last release.

### 2. Run the Release Command

Choose the appropriate release type:

```shell
# Patch release (0.1.0 -> 0.1.1)
make patch-release

# Minor release (0.1.0 -> 0.2.0)
make minor-release

# Major release (0.1.0 -> 1.0.0)
make major-release
```

This command will:
1. Batch all unreleased changelog entries into `changes/vX.Y.Z.md`
2. Update `CHANGELOG.md`
3. Update version in `Chart.yaml`
4. Update image tag in `values.yaml`
5. Regenerate documentation
6. Create `release/vX.Y.Z` branch
7. Commit all changes
8. Tag `vX.Y.Z-rc.1` (release candidate)
9. Push branch and tag
10. Print URL to open the release PR

### 3. Open the Release PR

Click the URL printed by the release command to open a pull request from the release branch to main.

## RC Testing

The RC tag triggers a GitHub Actions workflow that:
- Builds the `pgedge-helm-utils` Docker image with the RC tag (includes provenance and SBOM)
- Packages and signs the Helm chart with GPG
- Creates a GitHub Release (marked as pre-release)
- Attaches the signed Helm chart (`.tgz`) and provenance file (`.prov`)

### Testing with the RC Image

To test the RC, override the image tag when installing:

```shell
helm install pgedge ./ \
  --values examples/configs/single/values.yaml \
  --set pgEdge.initSpockImageName=ghcr.io/pgedge/pgedge-helm-utils:vX.Y.Z-rc.1
```

### Creating Additional RCs

If fixes are needed during the RC phase:

1. Push fixes to the `release/vX.Y.Z` branch
2. Manually tag the next RC:

```shell
git tag -a vX.Y.Z-rc.2 -F changes/vX.Y.Z.md
git push origin vX.Y.Z-rc.2
```

## Merging the Release

Once testing is complete and the PR is approved:

1. Merge the PR to main
2. A GitHub Action automatically tags the final release (`vX.Y.Z`)
3. Another workflow builds the final Docker image and creates the GitHub Release

## Chart Signing

All released Helm charts are signed with GPG. Each release includes:
- `pgedge-vX.Y.Z.tgz` - The packaged Helm chart
- `pgedge-vX.Y.Z.tgz.prov` - The provenance file (GPG signature)

### Verifying Chart Signature

To verify a chart's signature:

```shell
# Download the chart and provenance file from the GitHub release
helm verify pgedge-vX.Y.Z.tgz
```

Note: You need the public key in your keyring to verify signatures.

## Troubleshooting

### changie not found

Install changie from https://changie.dev/guide/installation/

### No unreleased changes

If `changes/unreleased/` is empty, create entries with `changie new` before releasing.

### Release notes not found

The tag-release workflow requires `changes/vX.Y.Z.md` to exist. This is created automatically by `changie batch` during the release process.
