{
  description = "Go dev shell";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs {
          inherit system;
        };
      in
      {
        devShells.default = pkgs.mkShellNoCC {
          packages = [
            pkgs.go_1_26
            pkgs.gopls
            pkgs.gotools
            pkgs.postgresql_18
          ];

          shellHook = '' # bash
            export GOPATH="$PWD/.gopath"
            export GOMODCACHE="$PWD/.gomodcache"
            export PATH="$GOPATH/bin:$PATH"
            export CGO_ENABLED=0
            mkdir -p "$GOPATH" "$GOMODCACHE"
          '';
        };
      });
}