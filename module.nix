# Home-Manager module for wallforge.
#
# Usage (in your HM flake):
#
#   inputs.wallforge.url = "path:/home/vaniello/Desktop/projects/wallforge";
#
#   outputs = { self, home-manager, wallforge, ... }: {
#     homeConfigurations.vaniello = home-manager.lib.homeManagerConfiguration {
#       modules = [
#         wallforge.homeManagerModules.default
#         ({ pkgs, ... }: {
#           nixpkgs.overlays = [ wallforge.overlays.default ];
#           programs.wallforge = {
#             enable = true;
#             shuffle = {
#               enable = true;
#               type   = "video";
#               interval = "15min";
#             };
#             settings.wpe.fps = 20;
#           };
#         })
#       ];
#     };
#   };
{
  config,
  lib,
  pkgs,
  ...
}:

let
  cfg = config.programs.wallforge;

  renderSettings = cfg.settings != { };

  settingsFile = pkgs.writeText "wallforge-config.json" (builtins.toJSON cfg.settings);

  baseArgs =
    if cfg.shuffle.enable then
      [
        "shuffle"
        "--interval=${cfg.shuffle.interval}"
      ]
      ++ (lib.optional (cfg.shuffle.type != null) "--type=${cfg.shuffle.type}")
      ++ cfg.shuffle.ids
    else if cfg.default != null then
      [
        "apply"
        cfg.default
      ]
    else
      null;
in
{
  options.programs.wallforge = {
    enable = lib.mkEnableOption "wallforge wallpaper manager";

    package = lib.mkOption {
      type = lib.types.package;
      default = pkgs.wallforge;
      defaultText = lib.literalExpression "pkgs.wallforge";
      description = ''
        The wallforge package to use. Defaults to `pkgs.wallforge`,
        which requires the wallforge overlay or a matching package in
        nixpkgs.
      '';
    };

    default = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "2995323628";
      description = ''
        Workshop ID or filesystem path to apply on graphical session
        start. Ignored when `shuffle.enable = true`.
      '';
    };

    shuffle = {
      enable = lib.mkEnableOption "cycling through a playlist via systemd";

      ids = lib.mkOption {
        type = lib.types.listOf lib.types.str;
        default = [ ];
        example = [
          "1116273880"
          "2832168392"
        ];
        description = ''
          Explicit playlist of Workshop IDs or paths. When empty and
          `type` is set, the full subscription list of that type is
          used at runtime.
        '';
      };

      type = lib.mkOption {
        type = lib.types.nullOr (
          lib.types.enum [
            "video"
            "scene"
            "image"
            "web"
          ]
        );
        default = null;
        description = "Pick subscriptions of this WE type when `ids` is empty.";
      };

      interval = lib.mkOption {
        type = lib.types.str;
        default = "15min";
        example = "30s";
        description = "Go-style duration between rotations (30s, 5m, 1h).";
      };
    };

    settings = lib.mkOption {
      type = lib.types.attrs;
      default = { };
      example = lib.literalExpression ''
        {
          wpe.fps = 20;
          swww.transition = "wipe";
        }
      '';
      description = ''
        Contents of `~/.config/wallforge/config.json`. Only defined when
        non-empty; keys that are omitted fall back to wallforge defaults.
      '';
    };
  };

  config = lib.mkIf cfg.enable {
    home.packages = [ cfg.package ];

    xdg.configFile."wallforge/config.json" = lib.mkIf renderSettings {
      source = settingsFile;
    };

    # Unit is generated only when there's actually something to run.
    # Bare `enable = true` with no default and no shuffle installs the
    # binary and the config, leaving invocation up to the user.
    systemd.user.services.wallforge = lib.mkIf (baseArgs != null) {
      Unit = {
        Description = "Wallforge wallpaper manager";
        After = [ "graphical-session.target" ];
        PartOf = [ "graphical-session.target" ];
      };
      Service = {
        ExecStart = lib.escapeShellArgs ([ "${cfg.package}/bin/wallforge" ] ++ baseArgs);
        Restart = "on-failure";
        RestartSec = 5;
      };
      Install.WantedBy = [ "graphical-session.target" ];
    };
  };
}
