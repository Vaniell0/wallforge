{
  description = "Wallforge — unified wallpaper manager for Hyprland (swww + mpvpaper + linux-wallpaperengine + Steam Workshop)";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems
        (system: f (import nixpkgs { inherit system; }));

      mkProject = pkgs:
        let
          lib = pkgs.lib;

          src = lib.cleanSourceWith {
            src = ./.;
            filter = path: type:
              let b = builtins.baseNameOf path; in
              !(b == "result" || b == ".git" || b == ".direnv" || b == "dist");
          };

          linux-wallpaperengine = pkgs.callPackage ./nix/linux-wallpaperengine.nix { };

          runtimeDeps = with pkgs; [
            awww
            mpvpaper
            linux-wallpaperengine
          ];

          # buildGoModule fought us over internal/* packages (insisted on
          # -mod=vendor with no vendor dir). For a stdlib-only module,
          # a plain stdenv + `go build` is simpler and actually works.
          wallforge = pkgs.stdenv.mkDerivation {
            pname = "wallforge";
            version = "0.1.0-alpha";
            inherit src;

            nativeBuildInputs = [ pkgs.go pkgs.makeWrapper ];

            buildPhase = ''
              runHook preBuild
              export HOME=$TMPDIR
              export GOCACHE=$TMPDIR/gocache
              export GOPATH=$TMPDIR/gopath
              export GOFLAGS=-mod=mod
              export GOPROXY=off
              export GOWORK=off
              export CGO_ENABLED=0
              go build -trimpath -ldflags='-s -w' -o wallforge.bin ./cmd/wallforge
              runHook postBuild
            '';

            installPhase = ''
              runHook preInstall
              install -Dm755 wallforge.bin $out/bin/wallforge
              wrapProgram $out/bin/wallforge \
                --prefix PATH : ${lib.makeBinPath runtimeDeps}
              runHook postInstall
            '';

            meta = {
              description = "Unified wallpaper manager for Hyprland";
              homepage = "https://github.com/vaniello/wallforge";
              license = lib.licenses.asl20;
              platforms = lib.platforms.linux;
              mainProgram = "wallforge";
            };
          };
        in { inherit wallforge linux-wallpaperengine runtimeDeps; };

    in {
      packages = forAllSystems (pkgs:
        let p = mkProject pkgs; in {
          default = p.wallforge;
          linux-wallpaperengine = p.linux-wallpaperengine;
        });

      apps = forAllSystems (pkgs:
        let p = mkProject pkgs; in {
          default = { type = "app"; program = "${p.wallforge}/bin/wallforge"; };
        });

      devShells = forAllSystems (pkgs:
        let p = mkProject pkgs; in {
          default = pkgs.mkShell {
            packages = with pkgs; [
              go_1_25
              gopls
              gotools
              go-tools
              golangci-lint
              delve
            ] ++ p.runtimeDeps;

            shellHook = ''
              echo ""
              echo "Wallforge devShell ($(go version))"
              echo "  go run ./cmd/wallforge apply <path>   — apply wallpaper"
              echo "  go build ./cmd/wallforge              — build binary"
              echo "  nix build                             — reproducible build"
              echo ""
            '';
          };
        });

      formatter = forAllSystems (pkgs: pkgs.nixpkgs-fmt);

      # Home-Manager module and a nixpkgs overlay that exposes `pkgs.wallforge`
      # and `pkgs.linux-wallpaperengine`. Users import both in their own HM
      # flake; see module.nix for the usage recipe.
      homeManagerModules.default = import ./module.nix;

      overlays.default = final: prev: {
        wallforge = (mkProject final).wallforge;
        linux-wallpaperengine = (mkProject final).linux-wallpaperengine;
      };
    };
}
