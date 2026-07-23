@{
    # Single source of truth for pinned toolchain versions and downloads.
    #
    # Integrity is verified one of two ways, never skipped:
    #   Sha256           - a literal pinned checksum (offline-verifiable).
    #   Sha256Url + Match - the vendor's official checksum file is fetched over
    #                       the same TLS; the line whose filename matches Match
    #                       supplies the expected hash. This mirrors the proven
    #                       node bootstrap in scripts/setup.ps1 and avoids
    #                       hand-copied hashes drifting from the pinned file.
    #
    # Every Url and Sha256Url must be https (CI enforces this), and every
    # downloadable entry must carry exactly one of Sha256 or Sha256Url.

    Node = @{
        Version   = 'v24.16.0'
        Url       = 'https://nodejs.org/dist/v24.16.0/node-v24.16.0-win-x64.zip'
        Sha256Url = 'https://nodejs.org/dist/v24.16.0/SHASUMS256.txt'
        Match     = 'node-v24.16.0-win-x64.zip'
        DirName   = 'node-v24.16.0-win-x64'
    }

    Go = @{
        Version   = 'go1.26.0'
        Url       = 'https://go.dev/dl/go1.26.0.windows-amd64.zip'
        # go.dev publishes a per-file checksum text at <url>.sha256.
        Sha256Url = 'https://go.dev/dl/go1.26.0.windows-amd64.zip.sha256'
        Match     = ''  # the .sha256 endpoint is a bare hash, no filename column
        DirName   = 'go'
    }

    Wails = @{
        # Installed via `go install` (module@version), not a downloaded archive,
        # so it is pinned by module version rather than a file checksum.
        Version = 'v2.12.0'
        Module  = 'github.com/wailsapp/wails/v2/cmd/wails@v2.12.0'
    }

    Jdk = @{
        # Eclipse Temurin 17 (LTS) portable zip for the Android build only.
        Version   = '17.0.12+7'
        Url       = 'https://github.com/adoptium/temurin17-binaries/releases/download/jdk-17.0.12%2B7/OpenJDK17U-jdk_x64_windows_hotspot_17.0.12_7.zip'
        # Adoptium publishes a sibling .sha256.txt: "<hash>  <filename>".
        Sha256Url = 'https://github.com/adoptium/temurin17-binaries/releases/download/jdk-17.0.12%2B7/OpenJDK17U-jdk_x64_windows_hotspot_17.0.12_7.zip.sha256.txt'
        Match     = 'OpenJDK17U-jdk_x64_windows_hotspot_17.0.12_7.zip'
        DirName   = 'jdk-17.0.12+7'
    }

    AndroidCmdlineTools = @{
        # Android SDK command-line tools; sdkmanager fetches the rest. License
        # acceptance is an explicit prompt in build-android.ps1, never silent.
        # Google does not publish a stable sibling checksum file for this
        # archive, so pin the literal SHA-256 here. Refresh when the version
        # bumps: (Get-FileHash -Algorithm SHA256 <file>).Hash.ToLowerInvariant()
        Version = '11076708'
        Url     = 'https://dl.google.com/android/repository/commandlinetools-win-11076708_latest.zip'
        Sha256  = 'REPLACE_WITH_VERIFIED_CMDLINE_TOOLS_11076708_WIN_ZIP_SHA256'
        DirName = 'android-cmdline-tools'
    }

    # System-only dependencies: detected or instructed, never downloaded blind.
    Git = @{
        MinVersion = '2.40'
        Note       = 'You already have git (you cloned this repo). The installer only checks the version.'
    }

    WebView2 = @{
        Note = 'Evergreen WebView2 Runtime ships with Windows 11. If absent, install.ps1 offers a per-user install via winget or the Evergreen bootstrapper.'
    }
}
