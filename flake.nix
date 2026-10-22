{
  description = "Go project for simples3 with ListObjectsV2 support";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      supportedSystems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forEachSystem = f: nixpkgs.lib.genAttrs supportedSystems (system: f {
        pkgs = import nixpkgs {
          inherit system;
        };
      });
    in
    {
      devShells = forEachSystem ({ pkgs }: {
        default = pkgs.mkShell {
          buildInputs = with pkgs; [
            # Go development
            go
            gopls
            go-outline
            delve

            # Build tools and utilities
            just
            docker
            awscli2

            # Additional utilities
            curl
            jq
          ];

          # Environment variables for the shell
          shellHook = ''
            echo "🚀 SimpleS3 Development Environment Loaded"
            echo "Go version: $(go version)"
            echo "Available commands: just --list"
            export GOPATH=$(go env GOPATH)
            export PATH=$PATH:$GOPATH/bin

            # Shared public test-fixture settings for the CLI and the Go tests.
            source ${./scripts/garage-env.sh}
          '';
        };
      });
    };
}
