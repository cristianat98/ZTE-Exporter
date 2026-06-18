FROM golang:1.25-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/zte-exporter ./cmd

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/zte-exporter /zte-exporter

EXPOSE 9111
ENTRYPOINT ["/zte-exporter"]
