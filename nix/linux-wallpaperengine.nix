# Nix derivation for linux-wallpaperengine (Almamu/linux-wallpaperengine).
#
# STATUS: builds successfully as of 2026-04-23 against nixpkgs-unstable
# (c89b1b3...). `nix build .#linux-wallpaperengine` produces a working
# $out/bin/linux-wallpaperengine wrapper.
#
# Gotchas captured during the first successful build:
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
#
# 6. libcef.so carries undefined references to GLib/ATK/NSS/CUPS/libudev
#    symbols that Chromium resolves at runtime via its own dlopen. Nix's
#    binutils-wrapper enables -Wl,-z,defs/--no-undefined by default, which
#    turns those into hard link errors — NIX_LDFLAGS is set to
#    --unresolved-symbols=ignore-in-shared-libs to work around it.
#
# 7. cmake install scatters kissfft/quickjs/SPIRV-Cross demo binaries and a
#    SPIRV-Cross pkg-config file with a malformed path into the install
#    prefix. postInstall scrubs them, keeps $out/lib/libkissfft-float.so
#    (the main binary links against it), and lays out the real payload
#    under $out/opt/linux-wallpaperengine with a wrapper in $out/bin.
#
# 8. autoPatchelfHook + --chdir wrapper give us a self-contained tree
#    without /build/ RPATH leakage.
{
  lib,
  stdenv,
  fetchFromGitHub,
  fetchurl,
  cmake,
  pkg-config,
  wayland-scanner,
  python3,
  autoPatchelfHook,
  makeWrapper,
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
  libx11,
  libxrandr,
  libxinerama,
  libxcursor,
  libxi,
  libxxf86vm,
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
    hash = "sha256-JKwZgOYr57GuosM31r1Lx3DczYs35HxtuUs5fxPsTcY=";
  };
in

stdenv.mkDerivation {
  pname = "linux-wallpaperengine";
  version = "unstable-2026-04-23";

  src = fetchFromGitHub {
    owner = "Almamu";
    repo = "linux-wallpaperengine";
    rev = "be773361d078997afa0d5768951538c40b4b790c";
    hash = "sha256-OHgI4qFUbFpNUdT4sRbWxNPPvKxmBJExkinqEXz+UJo=";
    fetchSubmodules = true;
  };

  nativeBuildInputs = [
    cmake
    pkg-config
    wayland-scanner
    python3
    autoPatchelfHook
    makeWrapper
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
    libx11
    libxrandr
    libxinerama
    libxcursor
    libxi
    libxxf86vm
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

  # Pre-fetched CEF tarball is extracted into $TMPDIR and passed to cmake
  # via -DCEF_ROOT. DownloadCEF.cmake does IS_DIRECTORY "${CEF_ROOT}"
  # before attempting a download — since the directory exists, the
  # network path is skipped without needing to patch the function. The
  # `set(CEF_ROOT ... CACHE INTERNAL)` inside it also won't overwrite a
  # value already in the cache from -D.
  preConfigure = ''
    mkdir -p "$TMPDIR/lwpe-cef"
    tar -xjf ${cefSrc} -C "$TMPDIR/lwpe-cef"
    cmakeFlagsArray+=("-DCEF_ROOT=$TMPDIR/lwpe-cef/${cefArchive}")
  '';

  cmakeFlags = [
    "-DCMAKE_BUILD_TYPE=Release"
    "-DCMAKE_INSTALL_PREFIX=${placeholder "out"}/opt/linux-wallpaperengine"
    "-DCEF_DISTRIBUTION_TYPE=${cefVariant}"
    "-DCMAKE_SKIP_BUILD_RPATH=ON"
    "-Wno-dev"
  ];

  # libcef.so carries undefined refs to GLib/ATK/NSS/CUPS/libudev symbols
  # that Chromium resolves at runtime via dlopen. Nix's binutils-wrapper
  # sets -Wl,-z,defs/--no-undefined by default, which turns those into
  # hard link errors. Allow undefined symbols that live in shared deps.
  env.NIX_LDFLAGS = "--unresolved-symbols=ignore-in-shared-libs";

  # CEF ships libcef.so and a bunch of runtime data files that the main
  # binary loads from its own directory. Copy them alongside the binary in
  # $out/opt, then create a thin wrapper in $out/bin that cds into that
  # directory before exec'ing the real binary (same pattern as the Arch
  # PKGBUILD). Also drop the kissfft/quickjs demo binaries that upstream
  # `install` scatters — they aren't useful for end users and confuse the
  # $out layout.
  postInstall = ''
    lwpeDir=$out/opt/linux-wallpaperengine
    cp -r "$TMPDIR/lwpe-cef/${cefArchive}/Release"/* "$lwpeDir/"
    cp -r "$TMPDIR/lwpe-cef/${cefArchive}/Resources"/* "$lwpeDir/"

    # Prune demo/test binaries from the kissfft and quickjs subprojects
    # that leak into the install prefix root.
    rm -f "$lwpeDir"/{tkfc-float,tr-float,ffr-float,bm_fftw-float,testcpp-float}
    rm -f "$lwpeDir"/{fastconv-float,fastconvr-float,fft-float,psdpng-float,bm_kiss-float,st-float,fastfilt-float}
    rm -f "$lwpeDir"/bin/{qjs,qjsc,spirv-cross,fastconv-float,fastconvr-float,fft-float,psdpng-float,bm_kiss-float,st-float,fastfilt-float} || true
    rm -f "$lwpeDir"/lib/libkissfft-float.so* || true

    # cmake install uses PERMISSIONS OWNER_READ OWNER_WRITE WORLD_EXECUTE
    # which leaves owner without +x — fix that so makeWrapper/ELF tooling
    # can touch the binary.
    chmod +x "$lwpeDir/linux-wallpaperengine"

    # Drop subproject artefacts (glslang, SPIRV-Cross, kissfft headers,
    # quickjs docs, pkg-config files) that leak into the prefix. These are
    # useless to end users and SPIRV-Cross generates a .pc file with a
    # malformed path (double slash after the prefix) that trips the
    # nixpkgs .pc validator.
    rm -rf $out/lib/pkgconfig $out/lib/cmake
    rm -rf $out/include $out/share/doc $out/share/cmake
    rm -rf $out/bin
    mkdir -p $out/bin
    # NOTE: keep $out/lib/libkissfft-float.so* — the main binary links
    # against it via RPATH, removing it breaks autoPatchelf.

    mkdir -p $out/bin
    makeWrapper "$lwpeDir/linux-wallpaperengine" "$out/bin/linux-wallpaperengine" \
      --chdir "$lwpeDir"
  '';

  # libcef.so contains references to interpreter that autoPatchelf shouldn't
  # try to "fix" — CEF libs are self-contained Chromium binaries, any rpath
  # rewrite on them risks breaking the sandbox.
  autoPatchelfIgnoreMissingDeps = [
    "libcups.so.2"
    "libudev.so.1"
  ];
  dontAutoPatchelf = false;

  meta = with lib; {
    description = "Wallpaper Engine replacement for Linux (X11 + Wayland)";
    homepage = "https://github.com/Almamu/linux-wallpaperengine";
    license = licenses.gpl3Only;
    platforms = platforms.linux;
    maintainers = [ ];
    mainProgram = "linux-wallpaperengine";
  };
}
