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

RUN mkdir -p /data && chown -R promag:promag /data
USER promag
EXPOSE 8080
VOLUME ["/data"]

ENTRYPOINT ["/usr/local/bin/promag"]
CMD ["--cloud", "--addr", ":8080", "--data-dir", "/data"]
