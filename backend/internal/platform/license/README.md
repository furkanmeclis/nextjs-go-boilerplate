# platform/license — offline license core (phase 1: verifier + signing CLI)

Design: [`docs/design/license-core.md`](../../../../docs/design/license-core.md).

This package verifies an **author-signed, offline** deployment license. The
running app never calls out to check a license: a signed token is read from
`LICENSE_KEY`, and its Ed25519 signature is verified against a public key
embedded at build time.

**Phase 1 is observe-only.** The license is verified and logged at startup
(`license.Observe`), but nothing gates traffic yet, whatever `LICENSE_ENFORCE`
is set to. Enforcement (a `503` middleware, the suspend switch, and the
outbound control agent) lands in later phases, each behind the enforce flag so
`main` stays shippable.

## Token format

A compact, versioned, domain-separated token (minimal stdlib, no new deps):

```
lic.v1.<base64url(claims JSON)>.<base64url(Ed25519 signature)>
```

The signature covers the bytes `lic.v1.<claims>`, so the version/purpose is
bound into the signature and a license signature can never be replayed as some
other Ed25519 message (e.g. a future control command).

Claims: `app` (must equal `APP_NAME`), optional `deployment` label, `iat`,
optional `exp` (zero = never), optional `features`, and `key_id`.

## Keys and rotation

Public keys are embedded from `keys/*.pub.pem` (PKIX/PEM Ed25519). Each **file
is named `<key_id>.pub.pem`**; a token names its signer in the `key_id` claim
and the verifier selects the matching key. Holding a *set* lets an old key keep
verifying during rollover — add the new public key, issue under it, and retire
the old file once no live token references it.

`keys/lic-dev-2026.pub.pem` is a **bootstrap** key for local/staging use. Its
private half is **not** in the repo. Before issuing production licenses,
generate a real keypair, keep the private key offline, and commit the new
public key here.

> `*.pem` is gitignored repo-wide; `backend/.gitignore` re-includes only this
> directory's `*.pub.pem` files. The `.pub.pem` suffix is load-bearing: the
> `go:embed` glob matches only public keys, and a private `*.key.pem` left here
> stays both gitignored and un-embedded.

## Author-side CLI (`cmd/license`, not shipped to deployments)

```bash
# Generate a signing keypair (private key kept offline; commit the .pem here)
make license-keygen LIC_KEY_ID=lic-2026-01 LIC_OUT=./secrets

# Sign a license token for a deployment
make license-sign \
  LIC_APP=my-app LIC_DEPLOYMENT=acme-prod \
  LIC_FEATURES=billing,ai LIC_EXP=2027-01-01 \
  LIC_KEY_ID=lic-2026-01 LIC_KEY=./secrets/lic-2026-01.key.pem
# → prints LICENSE_KEY=lic.v1.… for the deployment's .env
```

Or invoke the binary directly: `go run ./cmd/license sign …` /
`go run ./cmd/license keygen …`.

## Configuration

| Env | Default | Meaning |
|-----|---------|---------|
| `LICENSE_KEY` | empty | Signed token. Empty + non-dev = logged "unlicensed". |
| `LICENSE_CONTROL_URL` | empty | Author control-plane WSS URL (phase 3). |
| `LICENSE_ENFORCE` | `false` | Off = verify + log only; on = gate `/v1` (phase 2+). |

`APP_ENV=development` grants a synthetic valid license with every feature
enabled; no key is needed locally.
