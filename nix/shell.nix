{
  mkShell,
  go,
  nodejs,
  wails,
  gtk3,
  webkitgtk_4_1,
  pkg-config,
  gopls,
  gotools,
  delve,
}:

mkShell {
  packages = [
    go
    nodejs
    wails
    gtk3
    webkitgtk_4_1
    pkg-config
    gopls
    gotools
    delve
  ];

  env.CGO_ENABLED = "1";

  shellHook = ''
    export GOFLAGS="-tags=webkit2_41"
  '';
}
