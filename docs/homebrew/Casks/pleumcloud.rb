cask "pleumcloud" do
  # Template for the tap repo (gachon-star-want/homebrew-pleumcloud,
  # Casks/pleumcloud.rb). On each release: update version, then fill
  # sha256 with `shasum -a 256 <dmg>`.
  version "0.4.0"
  sha256 "0000000000000000000000000000000000000000000000000000000000000000"

  url "https://github.com/gachon-star-want/pleumcloud/releases/download/v#{version}/PleumCloud-v#{version}-macOS-universal.dmg"
  name "PleumCloud"
  desc "All your free cloud storage, one drive"
  homepage "https://github.com/gachon-star-want/pleumcloud"

  # Unsigned app (D13): brew downloads carry no quarantine attribute, so
  # the cask sidesteps Gatekeeper without an xattr dance.
  app "PleumCloud.app"

  zap trash: "~/.pleumcloud"
end
