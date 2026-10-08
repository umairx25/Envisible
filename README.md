# Envisible

A secrets manager CLI that allows you to securely store your secrets in a
Git repo and share them with your team. All credentials remain under
your control: no Envisible backend or external secrets server is
involved. No need to paste secrets into Slack or accidentally commit a
plaintext `.env` file!


## Install

**macOS / Linux**

```sh
curl -fsSL https://raw.githubusercontent.com/umairx25/Envisible/main/scripts/install.sh | sh
```

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/umairx25/Envisible/main/scripts/install.ps1 | iex
```

Both scripts download the prebuilt binary for your OS/architecture from the
latest [GitHub release](https://github.com/umairx25/Envisible/releases),
verify its checksum, and place it on your `PATH`. Pin a version with
`ENVIS_VERSION=v1.0.0` (shell) or `$env:ENVIS_VERSION="v1.0.0"` (PowerShell).

Prefer to build from source (requires Go):

```sh
go build -o envis ./cmd/envis
```

Check your install with `envis version`.


## How it works

When you initialize a project for the first time, a single `.envis` file is
created in the repository. It is the source of truth for the encrypted
environment. This file only contains encrypted or public metadata, so it is safe to commit.

``` yaml
version: 1        # Envisible file format version
rotated: 0        # Number of times the DEK has been rotated

users:
  - id: alice-macbook # each device is a distinct identity
    status: active
    pk: ALICE_PUBLIC # alice's public key
    dek_enc: "DEK encrypted for Alice"

  - id: bob
    status: pending
    pk: BOB_PUBLIC
    dek_enc: ""   # Empty until access is granted

payload:
  nonce: "..."
  ciphertext: "encrypted secrets"
```

### Starting a new project

Initialize Envisible from the repository root:

``` bash
envis init
```

Add secrets:

``` bash
envis set DATABASE_URL
envis set STRIPE_KEY
```

Or import an existing dotenv file:

``` bash
envis import .env
```

### Sharing access

A new teammate clones the repository and runs:

``` bash
envis request
```

This generates a local device identity if needed and adds a pending
entry containing the device's public key to `.envis`. The pending entry
cannot decrypt anything because it has no encrypted DEK.

After that change is committed and pushed, an existing authorized user
pulls it and grants access:

``` bash
envis grant bob-macbook
```

Granting seals the existing DEK to Bob's public key and changes the
request to active. It does **not** require re-encrypting the secrets.

Bob then pulls the updated `.envis`. His local private key can recover
his encrypted copy of the DEK, giving that device access to the
environment.

### Revoking access

``` bash
envis revoke bob
```

This command removes Bob, generates a new DEK, re-encrypts the secrets, and seals the new DEK for every remaining active user.

## Architecture

#### Cryptography

Envisible uses envelope encryption.

The **Data Encryption Key (DEK)** is a randomly generated symmetric key
used to encrypt the project's actual secret data. Rather than encrypting
the full payload separately for every user, Envisible encrypts the data
with the DEK and then seals that much smaller DEK independently for
every authorized public key.

``` text
                     secrets
                        │
                        ▼
                       DEK
                        │
                        ▼
                encrypted payload

                       DEK
              ┌─────────┴─────────┐
              ▼                   ▼
       Alice public key      Bob public key
              │                   │
              ▼                   ▼
       encrypted DEK         encrypted DEK
        for Alice             for Bob
```

The current cryptographic primitives are:

-   **Device keypairs:** Curve25519 using NaCl `box`.
-   **DEK protection:** NaCl sealed boxes, allowing a DEK to be
    encrypted for a recipient using only that recipient's public key.
-   **Secrets payload:** NaCl `secretbox` using XSalsa20-Poly1305 under
    the DEK.

#### Notes

- Private keys are sensitive credentials and cannot be shared. They're stored locally in a protected identity file with `0600` permissions: ``~/.config/envis/identity.yaml``. The location can be overridden with `ENVIS_HOME` or `XDG_CONFIG_HOME`.

- Envisible installs a one-time shell hook watches the current directory. When the shell enters a directory belonging to an accessible Envisible project, the CLI decrypts the secrets in memory and emits the environment changes required by the shell.


### Threat model

Envisible is designed under the assumption that an attacker may obtain
the entire `.envis` file---or even the entire Git repository and its
history. Possessing encrypted state alone should not reveal the
plaintext secrets without an authorized private key.

The important security boundaries are:

-   **Repository exposure:** `.envis` contains ciphertext, public keys,
    nonces, and encrypted DEKs. It does not contain plaintext secrets,
    plaintext DEKs, or private keys.
-   **Device compromise:** A compromised authorized device may expose
    the secrets that device is allowed to use. Envisible does not
    attempt to protect secrets from a fully compromised endpoint.
-   **Private-key theft:** A stolen private key may allow an attacker to
    decrypt repositories for which that identity is still active. Revoke
    the affected device to rotate the DEK and prevent access to future
    state.
-   **Historical access:** Revocation protects future versions. A
    previously authorized user may still decrypt historical revisions
    they were authorized to access.
-   **Already exposed credentials:** Rotating the DEK does not
    invalidate a database password, API token, or other credential that
    an attacker already learned. Rotate that credential with its
    provider.
-   **Private-key loss:** If no authorized private key capable of
    recovering the DEK remains, the encrypted secrets cannot be
    recovered. This is a consequence of having no central recovery
    backend.
-   **Runtime exposure:** Once injected, secrets exist in the
    shell/process environment and can be read by processes launched from
    that environment. Automatic injection trades narrower process
    isolation for transparent local development.



## Commands


| Command | Description |
| --- | --- |
| `init [--id ID] [--no-hook]` | Initialize a project, or configure an authorized device for an existing `.envis` |
| `set [NAME [VALUE]]` | Create or update a secret (prompts for anything omitted) |
| `delete NAME` | Delete a secret |
| `get [NAME] [--show]` | Show secret names (masked); `--show` reveals values |
| `import [FILE]` | Import secrets from a dotenv file (prompts if omitted) |
| `export [FILE]` | Write secrets to a dotenv file (default `.env`, never overwrites) |
| `request` | Request access (writes a pending entry into `.envis`) |
| `requests` | List pending access requests |
| `grant [USERNAME [PUBLIC_KEY]]` | Grant a pending request, or add directly with a key |
| `revoke [USERNAME]` | Revoke a user (rotates the key) or drop a pending request |
| `rotate` | Generate a new repo key and re-seal it for active users |
| `users` | List users and their status |
| `status` | Show this device's identity and access state |
| `inject [SHELL]` / `uninject [SHELL]` | Emit statements that set/unset secrets (used by the hook) |
| `hook [bash\|zsh\|powershell]` | Print shell integration |

## Windows

Envis runs natively on Windows (PowerShell) as well as under WSL.

- **WSL:** behaves exactly like Linux — bash/zsh hook and `0600` key
  permissions. This is the simplest path.
- **Native PowerShell:** `envis init` detects PowerShell and installs the hook
  into your `$PROFILE`. The hook wraps your prompt so secrets load on entering
  an Envis directory and unload on leaving, same as on Unix. To wire it up
  manually:

  ```powershell
  envis hook powershell | Out-String | Invoke-Expression   # add to $PROFILE
  ```

  You can also load secrets into the current session on demand:

  ```powershell
  envis inject powershell | Invoke-Expression
  ```

**Security note on Windows:** the local private key
(`identity.yaml`) is written without an OS ACL — Unix `0600` permissions do
not translate to Windows. The key is not restricted from other accounts on the
machine the way it is on Unix. `envis` prints this warning when it creates a
new identity on Windows. Protect the file and your user account accordingly.


## Releasing (maintainers)

Releases are automated by GitHub Actions (`.github/workflows/release.yml`).
Pushing a version tag builds binaries for macOS, Linux, and Windows (amd64 and
arm64), generates `SHA256SUMS`, and publishes a GitHub Release with all assets.

```sh
git tag v1.0.0
git push origin v1.0.0
```

The version is embedded into the binary via `-ldflags` and shown by
`envis version`. The install scripts always fetch the latest release unless
`ENVIS_VERSION` is set.



