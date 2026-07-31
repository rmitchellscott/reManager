{
  description = "Linux, MacOS, and Windows desktop app for managing mods on reMarkable tablets";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    flake-utils.lib.eachSystem [ "x86_64-linux" "aarch64-linux" ] (
      system:
      let
        pkgs = import nixpkgs { inherit system; };
        version = self.shortRev or self.dirtyShortRev or "dev";
      in
      {
        packages.default = pkgs.callPackage ./nix/package.nix { inherit version; };

        apps.default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/reManager";
        };

        devShells.default = pkgs.callPackage ./nix/shell.nix { };

        formatter = pkgs.nixfmt-rfc-style;
      }
    );
}
