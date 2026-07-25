FROM golang:1.25-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/zte-exporter ./cmd

# distroless has no shell/useradd, so write minimal passwd/group entries
# here and copy just those into the final image (not the build image's
# full system account list).
RUN echo 'zte-exporter:x:65532:65532::/:/usr/sbin/nologin' > /etc/passwd.zte-exporter && \
    echo 'zte-exporter:x:65532:' > /etc/group.zte-exporter

FROM gcr.io/distroless/static-debian12@sha256:d093aa3e30dbadd3efe1310db061a14da60299baff8450a17fe0ccc514a16639
COPY --from=build /etc/passwd.zte-exporter /etc/passwd
COPY --from=build /etc/group.zte-exporter /etc/group
COPY --from=build /out/zte-exporter /zte-exporter
USER zte-exporter:zte-exporter

EXPOSE 9111
ENTRYPOINT ["/zte-exporter"]
