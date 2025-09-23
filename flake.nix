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

            # Local S3 (MinIO) configuration for testing
            export AWS_S3_BUCKET=testbucket
            export AWS_S3_ENDPOINT=http://localhost:9000
            export AWS_S3_REGION=us-east-1
            export AWS_EC2_METADATA_DISABLED=true

            # AWS CLI credentials for MinIO
            export AWS_ACCESS_KEY_ID=minioadmin
            export AWS_SECRET_ACCESS_KEY=minioadmin

            # Simples3 library credentials
            export AWS_S3_ACCESS_KEY=minioadmin
            export AWS_S3_SECRET_KEY=minioadmin
          '';
        };
      });
    };
}