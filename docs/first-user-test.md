# First-user test

WOL needs people who have never used it to install it and wake a machine using
only the published docs. If you have two computers on the same network and
about 30 minutes, this test helps the project reach public beta (#6).

Follow the docs as written. When something is unclear, missing or wrong, write
it down and keep going if you can. That friction is the result we need. Do not
ask the maintainer for help until you have finished or are stuck.

## What you need

- A computer to run WOL: macOS, Linux or Windows.
- A second computer to wake, wired to the same network, with Wake-on-LAN
  enabled in its firmware and network adapter. Sleep is easier to test than
  shutdown.
- A terminal you normally use. Note its name and version.

## Steps

Record the time each step takes and anything that surprised you.

1. **Install.** Download the release archive for your system from the
   [latest release](https://github.com/aklkbqx/wol/releases/latest), check it
   against `SHA256SUMS`, extract it and put `wol` on your PATH, following the
   README. Run `wol version` and `wol doctor`.
2. **Optional: check the signature.** Follow
   [release signing](release-signing.md) to verify `SHA256SUMS.sig`, or run
   `wol update --check`.
3. **Add the second machine.** Follow
   [Homelab quick start: First machine](homelab.md#first-machine). Use
   discovery (`n`) if it finds the machine, or add it by hand (`a`).
4. **Check it.** With the machine awake, press `s`. It should show as online.
5. **Wake it.** Put the machine to sleep, wait until it is fully asleep, then
   wake it from the Wake Desk. Note how long it took to come back and what
   WOL showed.
6. **Use the CLI.** Run `wol status <name>` and
   `wol wake <name> --verify`, with the machine asleep again.
7. **Export.** Run `wol export > inventory.json`, then delete the machine in
   the Wake Desk (`d`) and restore it with `wol import inventory.json`.
8. **Exit.** Leave the Wake Desk with `q`, open it again and leave with Ctrl+C.
   Both should return you to a normal prompt.

Optional, if you use them: a router relay (OpenWrt with `etherwake`), a remote
session (`p`, then Enter), the `WOL_TUI_ASCII=1`, `NO_COLOR=1` or
`WOL_TUI_REDUCED_MOTION=1` settings.

## Report

Open an issue titled `First-user test: <OS> <architecture>` and paste this,
filled in. Leave out IP addresses, MAC addresses, host names and anything else
that identifies your network.

```text
WOL version:
OS and version / architecture:
Terminal and version:
Install method (release archive / make install / other):
Wake target: OS, sleep or shutdown, wired or Wi-Fi

Step results (pass / fail / skipped, minutes, notes):
1 Install:
2 Signature:
3 Add machine:
4 Check:
5 Wake:
6 CLI:
7 Export/import:
8 Exit:

Where the docs were unclear, missing or wrong:
Anything you had to work out yourself:
Would you keep using it? Why or why not:
```

The maintainer adds the result to #6 and fixes the docs where you got stuck.
