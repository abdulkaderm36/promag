ARG GO_VERSION=1.26

FROM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/promag .

FROM debian:bookworm-slim
RUN useradd --system --create-home --home-dir /home/promag promag
COPY --from=build /out/promag /usr/local/bin/promag

# The working directory holds ./data, which is the persistent volume. The default
# --data-dir is ./data, so no command needs to pass it explicitly.
WORKDIR /app
RUN mkdir -p /app/data && chown -R promag:promag /app
USER promag
EXPOSE 8080
VOLUME ["/app/data"]

ENTRYPOINT ["/usr/local/bin/promag"]
CMD ["--cloud", "--addr", ":8080"]
