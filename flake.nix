{
  description = "Development environment for PodGist";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      supportedSystems = [ "x86_64-linux" ];
      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;
    in {
      devShells = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
        in {
          default = pkgs.mkShell {
            packages = with pkgs; [
              python313
              ffmpeg
            ];

            shellHook = '' # bash
              echo "🎧 PodGist dev environment! 🚀"
              
              if [ ! -d "venv" ]; then
                echo "Creating new venv..."
                python -m venv venv
              fi
              
              source venv/bin/activate
              
              if [ -f "requirements.txt" ]; then
                echo "Syncing Python dependencies..."
                pip install --quiet --upgrade pip
                pip install --quiet -r requirements.txt
              else
                echo "⚠️ requirements.txt not found!"
              fi

              
              echo "Done!"
            '';
          };
        });
    };
}
