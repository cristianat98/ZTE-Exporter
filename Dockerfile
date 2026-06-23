FROM golang:1.25-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/zte-exporter ./cmd

FROM gcr.io/distroless/static-debian12@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639
COPY --from=build /out/zte-exporter /zte-exporter
USER nonroot:nonroot

EXPOSE 9111
ENTRYPOINT ["/zte-exporter"]
