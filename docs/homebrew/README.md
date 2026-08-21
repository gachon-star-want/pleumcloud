# Homebrew tap — setup (one-time)

The cask mitigates the unsigned-dmg Gatekeeper prompt (D13c): files
downloaded by brew carry no quarantine attribute.

1. Create the tap repo: `gachon-star-want/homebrew-pleumcloud` (public).
2. Copy this directory's `Casks/` into it.
3. Users install with:
   `brew install --cask gachon-star-want/pleumcloud/pleumcloud`

## Per release

1. `version` ← the new tag (without `v`).
2. `sha256` ← `shasum -a 256 PleumCloud-v<tag>-macOS-universal.dmg`
   from the release assets.
3. Commit to the tap repo.
