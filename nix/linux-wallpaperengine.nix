# Nix derivation for linux-wallpaperengine (Almamu/linux-wallpaperengine).
#
# STATUS: DRAFT / UNBUILT. The fakeHash values below will be replaced by the
# real ones the first time `nix build` is attempted — nix will print the
# correct hash in the error message. See README section "Building lwpe".
#
# Gotchas captured during research:
#
# 1. The project pulls Chromium Embedded Framework (CEF) at build time via
#    `file(DOWNLOAD)` in CMakeModules/DownloadCEF.cmake (~200 MB). The Nix
#    sandbox has no network access, so we fetch the CEF tarball as its own
#    derivation with fetchurl and point CEF_ROOT at the pre-extracted dir,
#    then skip the download block by dropping `DownloadCEF(...)` from
#    CMakeLists.txt. This only works if CEF versions stay in sync; when
#    lwpe bumps CEF upstream, both `cefVersion` here and the `hash` must be
#    updated together.
#
# 2. The repo uses git submodules (glslang, SPIRV-Cross, quickjs, kissfft,
#    argparse, stb, json, MimeTypes, Catch2). `fetchFromGitHub` with
#    `fetchSubmodules = true` handles that.
#
# 3. Default CMAKE_INSTALL_PREFIX is /opt/linux-wallpaperengine — overridden
#    to $out below.
#
# 4. Wayland support is enabled when wayland, wayland-protocols, wayland-egl
#    and wayland-scanner are present. All included in nativeBuildInputs /
#    buildInputs.
#
# 5. Legacy GL + GLEW + FreeGLUT are required even on modern Wayland builds
#    (the project predates Wayland).
{
  lib,
  stdenv,
  fetchFromGitHub,
  fetchurl,
  cmake,
  pkg-config,
  wayland-scanner,
  SDL2,
  glew,
  freeglut,
  glm,
  glfw,
  zlib,
  lz4,
  ffmpeg,
  mpv-unwrapped,
  libpulseaudio,
  fftw,
  libGL,
  libxkbcommon,
  wayland,
  wayland-protocols,
  xorg,
  nss,
  nspr,
  at-spi2-core,
  cups,
  libxcomposite,
  libxdamage,
  libxkbfile ? null,
  dbus,
  gtk3,
  pango,
  cairo,
  gdk-pixbuf,
  glib,
  expat,
  libdrm,
  mesa,
  libxshmfence,
  alsa-lib,
}:

let
  cefVersion = "135.0.17+gcbc1c5b+chromium-135.0.7049.52";
  cefVariant = "minimal";
  cefPlatform = "linux64";
  cefArchive = "cef_binary_${cefVersion}_${cefPlatform}_${cefVariant}";
  cefUrl = "https://cef-builds.spotifycdn.com/${lib.replaceStrings [ "+" ] [ "%2B" ] "${cefArchive}.tar.bz2"}";

  cefSrc = fetchurl {
    url = cefUrl;
    hash = lib.fakeHash;
  };
in

stdenv.mkDerivation {
  pname = "linux-wallpaperengine";
  version = "unstable-2026-04-23";

  src = fetchFromGitHub {
    owner = "Almamu";
    repo = "linux-wallpaperengine";
    rev = "be773361d078997afa0d5768951538c40b4b790c";
    hash = lib.fakeHash;
    fetchSubmodules = true;
  };

  nativeBuildInputs = [
    cmake
    pkg-config
    wayland-scanner
  ];

  buildInputs = [
    SDL2
    glew
    freeglut
    glm
    glfw
    zlib
    lz4
    ffmpeg
    mpv-unwrapped
    libpulseaudio
    fftw
    libGL
    libxkbcommon
    wayland
    wayland-protocols
    xorg.libX11
    xorg.libXrandr
    xorg.libXinerama
    xorg.libXcursor
    xorg.libXi
    xorg.libXxf86vm
    # CEF runtime dependencies (browser sandbox, GTK integration, audio):
    nss
    nspr
    at-spi2-core
    cups
    libxcomposite
    libxdamage
    dbus
    gtk3
    pango
    cairo
    gdk-pixbuf
    glib
    expat
    libdrm
    mesa
    libxshmfence
    alsa-lib
  ];

  # Drop into place the pre-fetched CEF tarball so CMake can skip the
  # build-time download. The path structure mirrors what DownloadCEF.cmake
  # expects: $download_dir/$CEF_DISTRIBUTION/.
  postUnpack = ''
    mkdir -p "$sourceRoot/cef-cache"
    tar -xjf ${cefSrc} -C "$sourceRoot/cef-cache"
  '';

  postPatch = ''
    # Comment out the live download; CEF_ROOT will be set via cmakeFlags.
    substituteInPlace CMakeModules/DownloadCEF.cmake \
      --replace 'file(DOWNLOAD' '# file(DOWNLOAD' \
      --replace 'execute_process(' '# execute_process(' || true
  '';

  cmakeFlags = [
    "-DCMAKE_BUILD_TYPE=Release"
    "-DCMAKE_INSTALL_PREFIX=${placeholder "out"}"
    "-DCEF_ROOT=${placeholder "NIX_BUILD_TOP"}/source/cef-cache/${cefArchive}"
    "-Wno-dev"
  ];

  meta = with lib; {
    description = "Wallpaper Engine replacement for Linux (X11 + Wayland)";
    homepage = "https://github.com/Almamu/linux-wallpaperengine";
    license = licenses.gpl3Only;
    platforms = platforms.linux;
    maintainers = [ ];
    mainProgram = "linux-wallpaperengine";
  };
}
