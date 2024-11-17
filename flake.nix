{
  description = "A Go CLI AliceTraINT training module for PIDML project";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
    let
      pkgs = import nixpkgs { inherit system; };
    in
    {
      devShell = pkgs.mkShell {
        buildInputs = with pkgs; [
          go_1_22
          docker
          golangci-lint
          makeWrapper
        ];

        shellHook = ''
          export PATH=$PWD/bin:$PATH
        '';
      };
    });
}
