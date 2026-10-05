# Sample files

These files were made with the app's own export features. Download one and
import it in the app; you don't have to change any settings by hand.

## `exceptional-scan-2026-09-21.json` — exceptional scan results

Trade prices for bases with extra sockets or 21%+ quality. **All 1203 keys
were scanned**; 541 had listings and 194 were above 75 exalted. League:
Forbidden Rites.

Running this scan from scratch takes hours, because searches are spaced about
90 seconds apart to stay within the trade API's search quota (600 per 6 hours).
Importing the file skips that wait.

**How to import:** Settings → Share scan results → **Import**.

Merge rule: for each key, **the newer scan wins**. Fresh records from your own
scan are not overwritten; only missing or older ones are updated. A file from
a different league is rejected.

## `profile-default.json` — sample profile

Every setting of a Simulacrum farming setup: 75 exalted threshold, NeverSink
Uber Plus Strict base, item groups, colours and sounds.

**How to import:** Settings → Profiles → **Import**. A profile carries *all*
settings, including the league and the in-game filter name. After importing,
the panel tells you if the league or filter name changed; in that case pick the
right filter in the game under Options → Item Filter.

## Prices go stale

Scan results reflect the market on the day they were taken. The app keeps
scanning in the background and refreshes old records over time, but if you
start from a very old file some prices may lag behind for the first few days.
