#!/usr/bin/env ruby
# frozen_string_literal: true

# Generates the Homebrew formula for archfit from a SHA256SUMS file.
# Usage: ruby scripts/update_homebrew_formula.rb <version> <sha256sums_file>
#
# Expects SHA256SUMS produced by `sha256sum archfit-v<version>-*` in the
# release workflow. Writes the complete formula to stdout.
#
# Example:
#   ruby scripts/update_homebrew_formula.rb 0.5.1 SHA256SUMS

abort "Usage: #{$PROGRAM_NAME} <version> <sha256sums_file>" unless ARGV.size == 2

version, sums_file = ARGV

sums = File.readlines(sums_file, chomp: true).to_h do |line|
  sha, name = line.split(/\s+/, 2)
  [name.strip, sha.strip]
end

def sha!(sums, version, platform)
  sums.fetch("archfit-v#{version}-#{platform}") do
    abort "SHA256SUMS: missing entry for archfit-v#{version}-#{platform}"
  end
end

# Note: \#{ ... } in this interpolated heredoc emits a literal #{...} in the
# output, which Homebrew's Ruby then interpolates at install time.
puts <<~FORMULA
  class Archfit < Formula
    desc "Architecture-fitness checks for AI agents and CI"
    homepage "https://github.com/alexei-led/archfit"
    version "#{version}"
    license "Apache-2.0"

    on_macos do
      if Hardware::CPU.arm?
        url "https://github.com/alexei-led/archfit/releases/download/v\#{version}/archfit-v\#{version}-darwin-arm64"
        sha256 "#{sha!(sums, version, "darwin-arm64")}"
      else
        url "https://github.com/alexei-led/archfit/releases/download/v\#{version}/archfit-v\#{version}-darwin-amd64"
        sha256 "#{sha!(sums, version, "darwin-amd64")}"
      end
    end

    on_linux do
      if Hardware::CPU.arm?
        url "https://github.com/alexei-led/archfit/releases/download/v\#{version}/archfit-v\#{version}-linux-arm64"
        sha256 "#{sha!(sums, version, "linux-arm64")}"
      else
        url "https://github.com/alexei-led/archfit/releases/download/v\#{version}/archfit-v\#{version}-linux-amd64"
        sha256 "#{sha!(sums, version, "linux-amd64")}"
      end
    end

    def install
      binary_name = if OS.mac?
        Hardware::CPU.arm? ? "archfit-v\#{version}-darwin-arm64" : "archfit-v\#{version}-darwin-amd64"
      else
        Hardware::CPU.arm? ? "archfit-v\#{version}-linux-arm64" : "archfit-v\#{version}-linux-amd64"
      end

      bin.install binary_name => "archfit"
    end

    test do
      assert_match "archfit version", shell_output("\#{bin}/archfit --version")
    end
  end
FORMULA
