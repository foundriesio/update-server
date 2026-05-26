FROM public.ecr.aws/docker/library/golang:1.24-bookworm as build

RUN apt update && apt install -y lshw && lshw -json > /tmp/lshw.txt

COPY go.* /dg-sat/
WORKDIR /dg-sat
RUN go mod download

COPY / /dg-sat
RUN make dg-sat

# -----------------------------------------------------
FROM public.ecr.aws/docker/library/python:3.12

COPY --from=build /dg-sat/bin/dg-sat /usr/bin
COPY --from=build /tmp/lshw.txt /usr/share/

RUN pip install locust==2.43.4
