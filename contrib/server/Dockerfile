FROM public.ecr.aws/docker/library/golang:1.25 AS builder

# hack for building on QC managed laptops
RUN --mount=target=/ctx \
    if [ -d /ctx/.extra-certs/ ] ; then \
        cp /ctx/.extra-certs/* /usr/local/share/ca-certificates/ && update-ca-certificates; \
    fi

WORKDIR /src
COPY go.* ./
RUN go mod download

COPY . .
RUN go build \
    --ldflags '-linkmode external -extldflags "-static"' \
    -o fioserver \
    github.com/foundriesio/update-server/cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=builder /src/fioserver /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/fioserver"]
