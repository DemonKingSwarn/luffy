{
  description = "luffy - terminal movie/TV streamer";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f system);
    in
    {
      packages = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};

          version = "1.1.4";

          binaryName = {
            "x86_64-linux"  = "luffy-linux-amd64";
            "aarch64-linux" = "luffy-linux-arm64";
          }.${system};

          sha256 = {
            "x86_64-linux"  = "sha256-HGH3YeDAAsvbU2lgYQVcBx3JZKuu3rCMKdRI1unCmR0=";
            "aarch64-linux" = "sha256-jk1OQOR1XYXNxp4b6pw1eYhODFENupipeh+a8vmR/1Y=";
          }.${system};

        in
        {
          luffy = pkgs.stdenvNoCC.mkDerivation {
            pname = "luffy";
            inherit version;

            src = pkgs.fetchurl {
              url = "https://github.com/DemonKingSwarn/luffy/releases/download/v${version}/${binaryName}";
              inherit sha256;
            };

            dontUnpack = true;
            dontBuild = true;

            nativeBuildInputs = [ pkgs.makeWrapper ];

            # runtime deps luffy needs
            propagatedBuildInputs = with pkgs; [ mpv fzf yt-dlp ffmpeg chafa ];

            installPhase = ''
              install -Dm755 $src $out/bin/luffy
            '';

            postFixup = ''
              wrapProgram $out/bin/luffy \
                --prefix PATH : ${pkgs.lib.makeBinPath (with pkgs; [ mpv fzf yt-dlp ffmpeg chafa ])}
            '';

            meta = with pkgs.lib; {
              description = "Spiritual successor of flix-cli and mov-cli";
              homepage    = "https://github.com/DemonKingSwarn/luffy";
              license     = licenses.gpl3Only;
              maintainers = [ ];
              platforms   = [ "x86_64-linux" "aarch64-linux" ];
              mainProgram = "luffy";
            };
          };

          default = self.packages.${system}.luffy;
        }
      );

      apps = forAllSystems (system: {
        default = {
          type    = "app";
          program = "${self.packages.${system}.luffy}/bin/luffy";
        };
      });
    };
}
