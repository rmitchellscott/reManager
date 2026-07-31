{
  lib,
  buildGoModule,
  fetchNpmDeps,
  nodejs,
  wails,
  webkitgtk_4_1,
  pkg-config,
  npmHooks,
  autoPatchelfHook,
  wrapGAppsHook3,
  version,
}:

buildGoModule (finalAttrs: {
  pname = "remanager";
  inherit version;

  src = lib.cleanSource ../.;

  vendorHash = "sha256-J77/+IbSQHeAXvy3wZjUjuoPzcC133s45lMEiscG1s8=";

  env = {
    CGO_ENABLED = 1;
    npmDeps = fetchNpmDeps {
      src = "${finalAttrs.src}/frontend";
      hash = "sha256-uxWpVL5ynuNu2PtPqgyvoCo7CMRbXSHKVlx/VVVfS3Q=";
    };
    npmRoot = "frontend";
  };

  nativeBuildInputs = [
    wails
    pkg-config
    autoPatchelfHook
    wrapGAppsHook3
    nodejs
    npmHooks.npmConfigHook
  ];

  buildInputs = [ webkitgtk_4_1 ];

  buildPhase = ''
    runHook preBuild

    wails build -m -trimpath -tags webkit2_41 -ldflags "-X main.version=${finalAttrs.version}"

    runHook postBuild
  '';

  installPhase = ''
    runHook preInstall

    install -Dm755 build/bin/reManager $out/bin/reManager
    install -Dm644 flatpak/io.scottlabs.reManager.desktop $out/share/applications/io.scottlabs.reManager.desktop
    install -Dm644 flatpak/io.scottlabs.reManager.metainfo.xml $out/share/metainfo/io.scottlabs.reManager.metainfo.xml
    install -Dm644 assets/icon.svg $out/share/icons/hicolor/scalable/apps/io.scottlabs.reManager.svg

    runHook postInstall
  '';

  meta = {
    description = "Linux, MacOS, and Windows desktop app for managing mods on reMarkable tablets";
    homepage = "https://github.com/rmitchellscott/reManager";
    license = lib.licenses.gpl3Plus;
    mainProgram = "reManager";
    platforms = lib.platforms.linux;
  };
})
