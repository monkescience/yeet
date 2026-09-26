# Artifact verification

Release archives and the container image are signed with Sigstore and carry GitHub build provenance.
The examples use the Linux amd64 archive. Replace the file name for other platforms.

## Signatures

Download the archive and its `.sigstore.json` bundle from the same GitHub release, then run:

```sh
cosign verify-blob \
  --bundle yeet_linux_amd64.tar.gz.sigstore.json \
  --certificate-identity-regexp 'https://github.com/monkescience/yeet/.github/workflows/binaries.yaml@.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  yeet_linux_amd64.tar.gz
```

For the container image:

```sh
cosign verify \
  --certificate-identity-regexp 'https://github.com/monkescience/yeet/.github/workflows/image.yaml@.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/monkescience/yeet:v0.17.0 # x-yeet-version
```

## Provenance

```sh
gh attestation verify yeet_linux_amd64.tar.gz --repo monkescience/yeet
gh attestation verify oci://ghcr.io/monkescience/yeet:v0.17.0 --repo monkescience/yeet # x-yeet-version
```

Both commands confirm the artifact was built from `monkescience/yeet` and report the workflow and commit that produced it.
