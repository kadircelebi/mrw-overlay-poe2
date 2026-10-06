# Security policy

## Reporting a vulnerability

Please do **not** open a public issue for a security problem.

Report it privately through GitHub instead: **[Report a vulnerability](https://github.com/kadircelebi/mrw-overlay-poe2/security/advisories/new)** (the Security tab of this repository). Only the maintainer can see the report. English or Turkish is fine.

Helpful details:

- what is affected (app, browser extension, profile server or website) and the version,
- how to reproduce it, and what an attacker could do with it,
- a proof of concept, if you have one.

You will get an answer within a few days. Confirmed problems are fixed in a new release as soon as possible, and you are credited in the release notes unless you prefer otherwise. Please give the fix time to ship before talking about it publicly.

## Supported versions

Only the latest release is supported, from GitHub or the Microsoft Store. The GitHub version updates itself after you confirm (Settings → Updates); the Store keeps its version current.

## Scope

In scope:

- the desktop app (this repository),
- the browser extension "MrW Overlay for POE 2 Bridge" (`browser-extension/`),
- the public profile server, `profiles.mrwproject.com` (`cmd/profilesrv`),
- the website, `poe2.mrwproject.com`.

Out of scope: pathofexile.com and its trade site, Cloudflare, the price sources (poe.ninja, poe2scout), NeverSink's filter, denial-of-service and volume testing, and social engineering. Problems in those belong to their owners.

## Testing in good faith

Test against your own installation and your own data. Do not access other players' data or profiles, do not degrade the profile server or the website for others, and do not send automated traffic to pathofexile.com through the app. Research done within these limits will not be pursued.

## How the app protects you

So you know what to expect, and what would count as a vulnerability:

- **Your pathofexile.com session** reaches the app only from the browser extension, on `127.0.0.1` with a one-time connection code. It is stored encrypted with Windows (DPAPI) and sent only to `www.pathofexile.com`.
- **Updates** are downloaded from GitHub Releases and verified against the published SHA-256 before the exe is replaced. Releases are built by GitHub Actions from this repository.
- **Public profiles** are strictly validated on both the server and the app: fixed fields only, no unknown keys, size and character limits, and canonical re-encoding. A downloaded profile can change loot filter settings and nothing else; text from it can never add lines to the filter.
- **No telemetry.** The app sends nothing about you anywhere; see the [privacy policy](https://poe2.mrwproject.com/privacy/).
