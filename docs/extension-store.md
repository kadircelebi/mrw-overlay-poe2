# Browser extension: store listing

Everything the Chrome Web Store, Edge Add-ons and Firefox Add-ons (AMO) forms
ask for. Build the packages with:

```
go run ./cmd/packext
```

This writes `dist/extension/mrw-overlay-bridge-chrome-<version>.zip` (Chrome and
Edge) and `mrw-overlay-bridge-firefox-<version>.zip` (AMO). Both are built from
`browser-extension/`; the store packages carry no `key`, so each store assigns
its own ID. **After the first upload, copy the Chrome and Edge IDs into
`chromiumStoreIDs` in `browserlink.go`**, or the app will refuse the store
builds.

## Name and summary

- **Name:** MrW Overlay for POE 2 Bridge
- **Short description** (Chrome, max 132 characters):
  Connects your signed-in pathofexile.com session to the MrW Overlay desktop app on this computer, only when the app asks.
- **Category:** Tools (Chrome) / Other (AMO)
- **Language:** English
- **Homepage:** https://poe2.mrwproject.com/extension/
- **Support:** https://poe2.mrwproject.com/contact/
- **Privacy policy:** https://poe2.mrwproject.com/privacy/
- **Source code:** https://github.com/kadircelebi/poe2-filtre (MIT, folder `browser-extension/`)

## Detailed description

MrW Overlay for POE 2 is a free, open-source Windows app for Path of Exile 2: a
loot filter that follows current prices, and a price check that opens over the
game. Some of its features only work when the official trade site knows who
you are: live search, travel to the seller's hideout, and larger stat
searches.

The app never asks for your password. Instead, this extension hands over the
session of the browser where you are already signed in to pathofexile.com.

How it works:
1. In the app, you press "Connect browser" (Settings → Account). The app opens
   pathofexile.com with a one-time code in the address.
2. The extension checks that you are signed in, then sends your
   pathofexile.com session cookie to the app on this computer (127.0.0.1).
   Nothing is sent anywhere else.
3. The app stores it encrypted with Windows (DPAPI) and uses it only for the
   trade searches you make. Disconnecting in the app deletes it.

What it does not do:
- It does not read or change any page content, on pathofexile.com or anywhere
  else.
- It does not track browsing, collect analytics or talk to any server of ours.
- It does nothing until the app asks for a connection.

Not affiliated with or endorsed by Grinding Gear Games.

## Single purpose (Chrome)

Pass the user's pathofexile.com session to the MrW Overlay desktop app running
on the same computer, when the user starts a connection from that app, so the
app can make the user's own trade searches signed in.

## Permission justifications

- **cookies:** Reads the POESESSID cookie of www.pathofexile.com, the session
  the app needs. Only this one cookie of this one site is read, and only after
  the app starts a connection.
- **storage:** Keeps the one-time connection code for the few seconds (or
  minutes, if the user still has to sign in) until the connection finishes.
  Uses session storage, which is cleared when the browser closes.
- **Host permission https://www.pathofexile.com/\*:** The content script reads
  the one-time code from the address fragment when the app opens the site, and
  the background script checks the account page to tell whether the user is
  signed in. No page content is read or changed.
- **Host permission http://127.0.0.1:47819/\*:** The address of the desktop app
  on the user's own computer, where the session is delivered. The app only
  listens there while a connection is in progress.
- **Remote code:** No. All code is in the package; nothing is downloaded or
  evaluated.

## Data usage

**Chrome Web Store** ("What user data do you plan to collect"):
- Tick **Authentication information** (the session cookie is read and passed
  on). Nothing else.
- Certify all three: not sold to third parties; not used or transferred for
  purposes unrelated to the single purpose; not used for creditworthiness or
  lending.
- Explanation if asked: the cookie never leaves the user's computer; it goes to
  the user's own desktop app over 127.0.0.1.

**Edge Add-ons:** same answers as Chrome.

**Firefox (AMO):** the manifest declares `data_collection_permissions:
required: ["none"]`, because the session stays on the user's device (Mozilla
counts data sent to the developer or third parties). If a reviewer disagrees,
change it to `["authenticationInfo"]`; Firefox then shows it at install.

## Notes for reviewers

The extension works together with a desktop app, so a reviewer cannot see it
do anything without that app. Suggested text:

> This extension only acts when the MrW Overlay for POE 2 desktop app
> (https://github.com/kadircelebi/poe2-filtre) starts a connection: the app
> opens https://www.pathofexile.com/trade2#mrw-link=<code> and listens on
> 127.0.0.1:47819. Without the app running, the extension does nothing: the
> request to 127.0.0.1 fails and no data leaves the browser. The code is
> plain, unminified JavaScript (background.js, content.js).

## Images

- Icon 128×128: `browser-extension/icons/icon-128.png`
- Screenshots 1280×800 and the small promo tile 440×280: `docs/store/`
