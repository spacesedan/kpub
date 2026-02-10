FROM linuxserver/calibre:latest
WORKDIR /app
COPY kpub .
RUN mkdir -p /data/downloads /data/converted
ENTRYPOINT ["./kpub"]
CMD ["--config", "/data/config.yaml"]
