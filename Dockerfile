FROM linuxserver/calibre:latest
WORKDIR /app
COPY linux/amd64/kpub .
RUN mkdir -p /data/downloads /data/converted
ENTRYPOINT ["./kpub"]
CMD ["--config", "/data/config.yaml"]
