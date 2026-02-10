# Minimal image with Calibre CLI tools for ebook conversion.
# Based on Ubuntu 24.04 instead of linuxserver/calibre to avoid
# the full desktop environment (~500MB vs ~1.2GB).
# The official Calibre installer provides ebook-convert which handles
# epub, mobi, azw3 → kepub.epub conversion.
FROM ubuntu:24.04

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        wget ca-certificates python3 xz-utils xdg-utils \
        libegl1 libopengl0 libxcb-cursor0 libfreetype6 && \
    wget -nv -O- https://download.calibre-ebook.com/linux-installer.sh | sh /dev/stdin && \
    apt-get purge -y wget && \
    apt-get autoremove -y && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY linux/amd64/kpub .
RUN mkdir -p /data/downloads /data/converted
ENTRYPOINT ["./kpub"]
CMD ["--config", "/data/config.yaml"]
