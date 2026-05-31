FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata && rm -rf /var/lib/apt/lists/*
COPY notes-api /usr/local/bin/notes-api
EXPOSE 4000
CMD ["notes-api"]
