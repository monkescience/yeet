# Artifact verification

Release archives and the container image are signed through Sigstore keyless signing. Both also carry GitHub build provenance attestations. Replace the archive name when verifying another platform.

## Archive signature

Download an archive and its adjacent `.sigstore.json` bundle from the same GitHub release, then run:

```sh
cosign verify-blob \
  --bundle yeet_linux_amd64.tar.gz.sigstore.json \
  --certificate-identity-regexp 'https://github.com/monkescience/yeet/.github/workflows/binaries.yaml@.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  yeet_linux_amd64.tar.gz
```

This expects a certificate issued to `.github/workflows/binaries.yaml` in `monkescience/yeet` by GitHub Actions.

## Container signature

```sh
cosign verify \
  --certificate-identity-regexp 'https://github.com/monkescience/yeet/.github/workflows/image.yaml@.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/monkescience/yeet:v0.14.2 # x-yeet-version
```

This expects a certificate issued to `.github/workflows/image.yaml` in `monkescience/yeet` by GitHub Actions.

## Archive provenance

```sh
gh attestation verify yeet_linux_amd64.tar.gz --repo monkescience/yeet
```

This expects provenance from `.github/workflows/binaries.yaml` in `monkescience/yeet`. The repository selector verifies the source repository, and the result identifies the workflow and commit that produced the archive.

## Container provenance

```sh
gh attestation verify oci://ghcr.io/monkescience/yeet:v0.14.2 --repo monkescience/yeet # x-yeet-version
```

This expects provenance from `.github/workflows/image.yaml` in `monkescience/yeet` and reports the source workflow and commit.

## Related documentation

- [Documentation index](README.md)
- [First automated release](../README.md#quick-start)
- [CI setup](ci.md)
