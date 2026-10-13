#!/bin/sh
# Writes the Homebrew formula for a release from that release's SHA256SUMS, so the checksums in it are the ones the release signed.
# The formula goes in a tap (a repository named homebrew-<name>), and a person installs with `brew install <owner>/<name>/haas`.
#
#   deploy/homebrew/formula.sh v0.1.0-rc.2 dist > haas.rb
#   HAAS_RELEASE_BASE=file:///tmp/release deploy/homebrew/formula.sh v0.0.0-test /tmp/release > haas.rb   # a test build
set -eu
tag=${1:?the release, such as v0.1.0-rc.2}
dist=${2:?the folder that holds the SHA256SUMS}
base=${HAAS_RELEASE_BASE:-https://github.com/getlatentic/haas/releases/download/$tag}

sum() {
  line=$(grep " haas_${tag}_$1_$2.tar.gz\$" "$dist/SHA256SUMS") || { echo "SHA256SUMS has no haas_${tag}_$1_$2.tar.gz" >&2; exit 1; }
  echo "${line%% *}"
}

cat <<RUBY
class Haas < Formula
  desc "Run AI agents under common security, cost and policy controls, with evidence anybody can re-check"
  homepage "https://github.com/getlatentic/haas"
  version "${tag#v}"
  license "Apache-2.0"

  on_macos do
    on_arm do
      url "$base/haas_${tag}_darwin_arm64.tar.gz"
      sha256 "$(sum darwin arm64)"
    end
    on_intel do
      url "$base/haas_${tag}_darwin_amd64.tar.gz"
      sha256 "$(sum darwin amd64)"
    end
  end

  on_linux do
    on_arm do
      url "$base/haas_${tag}_linux_arm64.tar.gz"
      sha256 "$(sum linux arm64)"
    end
    on_intel do
      url "$base/haas_${tag}_linux_amd64.tar.gz"
      sha256 "$(sum linux amd64)"
    end
  end

  def install
    bin.install "haas"
    prefix.install "LICENSE", "NOTICE"
  end

  # brew services start haas: haas for one person on this computer, as a service of theirs.
  service do
    run [opt_bin/"haas", "local", "run"]
    keep_alive true
    log_path var/"log/haas.log"
    error_log_path var/"log/haas.log"
  end

  test do
    assert_match "haas", shell_output("#{bin}/haas version")
  end
end
RUBY
