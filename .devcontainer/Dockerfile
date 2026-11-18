# Wolfi base, pinned by digest. apk provides the Go 1.27 toolchain plus the
# X11/GL development headers the Debian xorg-dev/libgl1-mesa-dev packages
# covered, and gopls is installed on top.
FROM cgr.dev/chainguard/wolfi-base@sha256:e9b8c3739e5fb5fc778843a2ce74f8620588c9ddc9c44918cead527abcb0b628

RUN apk add --no-cache \
        go-1.27 \
        gcc \
        git \
        gettext \
        iptables \
        mesa \
        libglvnd-dev \
        libx11-dev \
        libxext-dev \
        libxrandr-dev \
        libxfixes-dev \
        libxi-dev \
        libxrender-dev \
        libxtst-dev \
        libxcursor-dev \
        libxinerama-dev \
    && go install -v golang.org/x/tools/gopls@latest


WORKDIR /app
