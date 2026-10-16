#!/bin/sh
# Writes the Homebrew formula for a release from that release's SHA256SUMS, so the checksums in it are the ones the release signed.
# The formula goes in a tap (a repository named homebrew-<name>), and a person installs with `brew install <owner>/<name>/gusto`.
#
#   deploy/homebrew/formula.sh v0.1.0-rc.2 dist > gusto.rb
#   GUSTO_RELEASE_BASE=file:///tmp/release deploy/homebrew/formula.sh v0.0.0-test /tmp/release > gusto.rb   # a test build
set -eu
tag=${1:?the release, such as v0.1.0-rc.2}
dist=${2:?the folder that holds the SHA256SUMS}
base=${GUSTO_RELEASE_BASE:-https://github.com/getlatentic/gusto/releases/download/$tag}

sum() {
  line=$(grep " gusto_${tag}_$1_$2.tar.gz\$" "$dist/SHA256SUMS") || { echo "SHA256SUMS has no gusto_${tag}_$1_$2.tar.gz" >&2; exit 1; }
  echo "${line%% *}"
}

cat <<RUBY
class Gusto < Formula
  desc "Run AI agents under common security, cost and policy controls, with evidence anybody can re-check"
  homepage "https://github.com/getlatentic/gusto"
  version "${tag#v}"
  license "Apache-2.0"

  on_macos do
    on_arm do
      url "$base/gusto_${tag}_darwin_arm64.tar.gz"
      sha256 "$(sum darwin arm64)"
    end
    on_intel do
      url "$base/gusto_${tag}_darwin_amd64.tar.gz"
      sha256 "$(sum darwin amd64)"
    end
  end

  on_linux do
    on_arm do
      url "$base/gusto_${tag}_linux_arm64.tar.gz"
      sha256 "$(sum linux arm64)"
    end
    on_intel do
      url "$base/gusto_${tag}_linux_amd64.tar.gz"
      sha256 "$(sum linux amd64)"
    end
  end

  def install
    bin.install "gusto"
    prefix.install "LICENSE", "NOTICE"
  end

  # brew services start gusto: gusto for one person on this computer, as a service of theirs.
  service do
    run [opt_bin/"gusto", "local", "run"]
    keep_alive true
    log_path var/"log/gusto.log"
    error_log_path var/"log/gusto.log"
  end

  test do
    assert_match "gusto", shell_output("#{bin}/gusto version")
  end
end
RUBY
