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

          runtimeDeps = with pkgs; [
            swww
            mpvpaper
          ];

          wallforge = pkgs.buildGoModule {
            pname = "wallforge";
            version = "0.1.0-alpha";
            inherit src;

            vendorHash = null;

            subPackages = [ "cmd/wallforge" ];

            nativeBuildInputs = [ pkgs.makeWrapper ];

            postInstall = ''
              wrapProgram $out/bin/wallforge \
                --prefix PATH : ${lib.makeBinPath runtimeDeps}
            '';

            meta = {
              description = "Unified wallpaper manager for Hyprland";
              homepage = "https://github.com/vaniello/wallforge";
              license = lib.licenses.asl20;
              platforms = lib.platforms.linux;
              mainProgram = "wallforge";
            };
          };
        in { inherit wallforge runtimeDeps; };

    in {
      packages = forAllSystems (pkgs:
        let p = mkProject pkgs; in {
          default = p.wallforge;
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
    };
}
