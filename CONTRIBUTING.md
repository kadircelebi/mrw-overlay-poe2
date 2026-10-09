# Contributing

Thanks for helping. English or Turkish is fine everywhere: issues, discussions and pull requests.

## Reporting a bug

Open an issue with the **Bug report** form. The most useful details:

- the app version (bottom of the panel, or Settings → Updates) and whether it is the Microsoft Store or the GitHub version,
- what happened and what you expected,
- for price check or market problems, the item text (Ctrl+C on the item in game; Ctrl+Alt+C copies the advanced text with tiers, which helps most),
- the language your game runs in, if it is not English (Options → Game → Language),
- screenshots (please blur other players' names),
- `%APPDATA%\PoE2Filtre\crash.log` if the app closed by itself.

Never paste files from `%APPDATA%\PoE2Filtre` other than `crash.log`: they can contain your pathofexile.com session.

Security problems go through a private report instead; see [SECURITY.md](SECURITY.md).

## Ideas and questions

Use the **Feature request** form, or [Discussions](https://github.com/kadircelebi/mrw-overlay-poe2/discussions) for questions. Describing the situation in game where the idea would help is worth more than a finished design.

## Pull requests

Please open an issue first, so the approach is agreed before you spend time on it. Then:

- keep a pull request to one change,
- run the tests before you push:

  ```bash
  go test ./...
  cd frontend && npm run check
  ```

- add or update tests for behaviour you change, especially filter rules and price or craft math,
- interface text lives in `frontend/src/lib/locales/` (English, Turkish, Traditional Chinese); a new string needs all three, `npm run check` verifies it,
- items copied from a game in another language belong in `internal/overlay/testdata/locale/<language>/` as test samples; the test there explains how to add one.

How to build and run the app is in the [README](README.md#for-developers).

Translations are very welcome, as are corrections to the existing ones.

## What the app will not do

The app only uses what the game and the official trade site offer every player. Changes that read or modify game memory, send input to the game beyond the user's own hotkey, or automate trading will not be accepted (see the Path of Exile terms of use).

## License

By contributing you agree that your contribution is released under the [MIT license](LICENSE) of this project.
