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
        lib = pkgs.lib;
        goDirective =
          let
            directives = builtins.filter (line: lib.hasPrefix "go " line) (lib.splitString "\n" (builtins.readFile ./go.mod));
          in
          if directives == [] then
            throw "go.mod does not contain a go directive"
          else
            builtins.head directives;
        goVersion = lib.removePrefix "go " goDirective;
        goVersionParts = builtins.match "([0-9]+)\\.([0-9]+)" goVersion;
        goPackageAttr =
          if goVersionParts == null then
            throw "unsupported Go version in go.mod: ${goVersion}"
          else
            "go_${builtins.elemAt goVersionParts 0}_${builtins.elemAt goVersionParts 1}";
        goPackage =
          if builtins.hasAttr goPackageAttr pkgs then
            builtins.getAttr goPackageAttr pkgs
          else
            throw "nixpkgs does not provide ${goPackageAttr}";
      in
      {
        devShells.default = pkgs.mkShellNoCC {
          packages = [
            goPackage
            pkgs.gopls
            pkgs.gotools
            pkgs.postgresql_18
          ];

          shellHook = '' # bash
            export GOPATH="$PWD/.gopath"
            export GOMODCACHE="$PWD/.gomodcache"
            export PATH="$GOPATH/bin:$PATH"
            export CGO_ENABLED=0
            export GO_VERSION="$(${pkgs.runtimeShell} ./scripts/go-version.sh)"
            export DOCKER_HOST=unix://$XDG_RUNTIME_DIR/podman/podman.sock
            export TESTCONTAINERS_RYUK_DISABLED=true
            mkdir -p "$GOPATH" "$GOMODCACHE"
          '';
        };
      });
}