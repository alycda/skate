# Skate

<p>
    <img src="https://stuff.charm.sh/skate/skate-header.png?2" width="480" alt="A nice rendering of a roller skate with the words ‘Charm Skate’ next to it"><br>
    <a href="https://github.com/charmbracelet/skate/releases"><img src="https://img.shields.io/github/release/charmbracelet/skate.svg" alt="Latest Release"></a>
    <a href="https://github.com/charmbracelet/skate/actions"><img src="https://github.com/charmbracelet/skate/workflows/build/badge.svg" alt="Build Status"></a>
</p>

A personal key-value store. 🛼

***
⚠️ As of v1.0.0 Skate operates locally and no longer syncs to the Charm Cloud. [Read more about why][sunset] and see [the v1.0.0 release notes](https://github.com/charmbracelet/skate/releases/tag/v1.0.0) for a migration guide. The Charm Cloud [sunsets][sunset] on 29 November 2024.

[sunset]: https://github.com/charmbracelet/charm?tab=readme-ov-file#sunsetting-charm-cloud
***

Skate is simple and powerful. Use it to save and retrieve anything you’d
like—even binary data.

```bash
# Store something (and sync it to the network)
skate set kitty meow

# Fetch something (from the local cache)
skate get kitty

# What’s in the store?
skate list

# Spaces are fine
skate set "kitty litter" "smells great"

# You can store binary data, too
skate set profile-pic < my-cute-pic.jpg
skate get profile-pic > here-it-is.jpg

# Unicode also works, of course
skate set 猫咪 喵
skate get 猫咪

# For more info
skate --help

# Do creative things with skate list
skate set penelope marmalade
skate set christian tacos
skate set muesli muesli

skate list | xargs -n 2 printf '%s loves %s.\n'
```

## Installation

Use a package manager:

```bash
# macOS or Linux
brew tap charmbracelet/tap && brew install charmbracelet/tap/skate

# Arch Linux (btw)
pacman -S skate

# Nix
nix-env -iA nixpkgs.skate

# Debian/Ubuntu
sudo mkdir -p /etc/apt/keyrings
curl -fsSL https://repo.charm.sh/apt/gpg.key | sudo gpg --dearmor -o /etc/apt/keyrings/charm.gpg
echo "deb [signed-by=/etc/apt/keyrings/charm.gpg] https://repo.charm.sh/apt/ * *" | sudo tee /etc/apt/sources.list.d/charm.list
sudo apt update && sudo apt install skate

# Fedora/RHEL
echo '[charm]
name=Charm
baseurl=https://repo.charm.sh/yum/
enabled=1
gpgcheck=1
gpgkey=https://repo.charm.sh/yum/gpg.key' | sudo tee /etc/yum.repos.d/charm.repo
sudo yum install skate
```

Or download it:

- [Packages][releases] are available in Debian and RPM formats
- [Binaries][releases] are available for Linux, macOS, and Windows

Or just install it with `go`:

```bash
go install github.com/charmbracelet/skate@latest
```

[releases]: https://github.com/charmbracelet/skate/releases

## Other Features

### List Filters

```bash
# list keys only
skate list -k

# list values only
skate list -v

# reverse lexicographic order
skate list -r

# add a custom delimeter between keys and values; default is a tab
skate list -d "\t"

# show binary values
skate list -b
```

### Databases

Sometimes you’ll want to separate your data into different databases:

```bash
# Database are automatically created on demand
skate set secret-boss-key@work-stuff password123

# Most commands accept a @db argument
skate set "office rumor"@work-stuff "penelope likes marmalade"
skate get "office rumor"@work-stuff
skate list @work-stuff

# Wait, what was that db named?
skate list-dbs
```

### Store Location

By default, Skate stores its databases in your operating system's user data
directory. You can choose a different parent directory with `--store`:

```bash
skate --store ~/.local/share/my-skate-store set kitty meow
skate --store ~/.local/share/my-skate-store get kitty
```

You can also set `SKATE_STORE` if you want every command in a shell session to
use the same store:

```bash
export SKATE_STORE=~/.local/share/my-skate-store
skate list-dbs
```

### Ditto Backend (experimental)

Skate can store its data in [Ditto](https://docs.ditto.live/) instead of
Badger, which lets it replicate between your devices. This is opt-in, and it
needs a build with the `ditto` tag: the SDK is a cgo wrapper around a native
library, and it is in Public Preview (`5.0.0-go-preview.3`). The default build
and the release binaries do not include it, and `--backend ditto` on one of
them tells you so.

**Platforms.** Linux (x86_64, aarch64) and macOS (aarch64) only, Go 1.24 or
later. There is no Windows support.

**Build.** Download the native library for your platform from
`https://software.ditto.live/go/Ditto/5.0.0-go-preview.3/dist/`
(`libdittoffi-linux-x86_64.tar.gz`, `libdittoffi-linux-aarch64.tar.gz` or
`libdittoffi-macos-aarch64.tar.gz`) and unpack it into `/usr/local/lib`, where
the Ditto docs say the SDK links it automatically. For another location, add
`-ldflags='-extldflags "-L/path/to/lib"'` to the build and set
`LD_LIBRARY_PATH` (Linux) or `DYLD_LIBRARY_PATH` (macOS) when running; see the
[Ditto Go install guide](https://docs.ditto.live/sdk/latest/install-guides/go).
Then build from a checkout:

```bash
CGO_ENABLED=1 go build -tags ditto -o skate .
```

**Configure.** Create a database in the Ditto Portal, then:

```bash
export SKATE_BACKEND=ditto                  # or pass --backend ditto
export SKATE_DITTO_DATABASE_ID=...          # your database ID
export SKATE_DITTO_URL=...                  # your database's server URL
export SKATE_DITTO_TOKEN=...                # development token; only `skate sync` needs it
```

Every command then works as usual against Ditto's local store, which lives in
`.ditto-<database id>` inside the Skate store directory (`--store` and
`SKATE_STORE` apply). All Skate databases (`@db`) live in one Ditto
collection, `skate_kv`.

**Syncing.** Writes are local until you sync. `skate sync` replicates with
Ditto until you press Ctrl-C, or for `--timeout`:

```bash
skate set kitty meow
skate sync --timeout 30s
```

Things to know:

- Ditto holds an exclusive lock on its directory, so while `skate sync` runs,
  other Skate commands on the Ditto backend fail with a "File already locked"
  error. Prefer `--timeout` over leaving it running.
- Skate does not know when replication has finished. `--timeout` is a fixed
  window, not a "sync until caught up".
- `skate delete-db` on the Ditto backend issues a Ditto `DELETE`, which
  replicates to peers on the next sync rather than only removing local data.
- Authentication uses Ditto's development provider, which is meant for
  prototypes. Production auth is not implemented.
- The SDK logs only errors by default. Set `SKATE_DITTO_LOG_LEVEL` to
  `warning`, `info`, `debug` or `verbose` when troubleshooting sync.
- Binary values are stored base64-encoded, so they are not readable in the
  Ditto Portal. Keys must be valid UTF-8.

**Testing.** `go test ./...` covers the default build. `go test -tags ditto
./...` also runs the backend against Ditto's local store and drives the CLI
with the scripts in `testdata/script-ditto`; it needs the native library on the
linker and loader paths. The `ditto` workflow does this in CI. Neither starts
real sync. To check your Portal setup before running `skate sync`, use the
[hurl](https://hurl.dev) files in `test/hurl`:

```bash
hurl --test --variable auth_url=<Auth URL> --variable database_id=<Database ID> \
  test/hurl/ditto-auth-reachable.hurl
```

`ditto-auth-reachable.hurl` needs no token. `ditto-auth-login.hurl` also takes
`--variable token=<development token>`; read the caveat at its top.

## Examples

Here are some of our favorite ways to use `skate`.

### Keep secrets out of your scripts

```bash
skate set gh_token GITHUB_TOKEN

#!/bin/bash
curl -su "$1:$(skate get gh_token)" \
    https://api.github.com/users/$1 \
    | jq -r '"\(.login) has \(.total_private_repos) private repos"'
```

### Keep passwords in their own database

```bash
skate set github@password.db PASSWORD
skate get github@password.db
```

### Use scripts to manage data

```bash
#!/bin/bash
skate set "$(date)@bookmarks.db" $1
skate list @bookmarks.db
```

What do you use `skate` for? [Let us know](mailto:vt100@charm.sh).

## Feedback

We’d love to hear your thoughts on this project. Feel free to drop us a note!

- [Twitter](https://twitter.com/charmcli)
- [The Fediverse](https://mastodon.social/@charmcli)
- [Discord](https://charm.sh/chat)

## License

[MIT](https://github.com/charmbracelet/skate/raw/main/LICENSE)

---

Part of [Charm](https://charm.sh).

<a href="https://charm.sh/"><img alt="The Charm logo" src="https://stuff.charm.sh/charm-badge.jpg" width="400"></a>

Charm热爱开源 • Charm loves open source
