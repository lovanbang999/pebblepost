# packaging/homebrew/pebblepost.rb
# Homebrew Formula for PebblePost CLI & Server

class Pebblepost < Formula
  desc "Lightweight, Git-friendly API client and runner for modern developers"
  homepage "https://github.com/lovanbang999/pebblepost"
  version "0.2.0"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/lovanbang999/pebblepost/releases/download/v#{version}/pebblepost_#{version}_darwin_arm64.tar.gz"
      # sha256 "REPLACE_WITH_SHA256_DARWIN_ARM64"
    else
      url "https://github.com/lovanbang999/pebblepost/releases/download/v#{version}/pebblepost_#{version}_darwin_amd64.tar.gz"
      # sha256 "REPLACE_WITH_SHA256_DARWIN_AMD64"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/lovanbang999/pebblepost/releases/download/v#{version}/pebblepost_#{version}_linux_arm64.tar.gz"
      # sha256 "REPLACE_WITH_SHA256_LINUX_ARM64"
    else
      url "https://github.com/lovanbang999/pebblepost/releases/download/v#{version}/pebblepost_#{version}_linux_amd64.tar.gz"
      # sha256 "REPLACE_WITH_SHA256_LINUX_AMD64"
    end
  end

  def install
    bin.install "pebblepost"
    bin.install "pebblepost-server" if File.exist?("pebblepost-server")
  end

  test do
    assert_match "PebblePost CLI Runner", shell_output("#{bin}/pebblepost version")
  end
end
