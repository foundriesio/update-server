FROM public.ecr.aws/docker/library/golang:1.24-bookworm as build

RUN apt update && apt install -y lshw ostree && \
	lshw -json > /tmp/lshw.txt && \
	ostree --repo=/tmp/ostree_repo init --mode=archive-z2 && \
	ostree --repo=/tmp/ostree_repo commit --branch=main --subject='scale-test ostree commit' --tree=dir=/usr && \
	ostree --repo=/tmp/ostree_repo summary -u

COPY go.* /dg-sat/
WORKDIR /dg-sat
RUN go mod download

COPY / /dg-sat
RUN make dg-sat

# -----------------------------------------------------
FROM public.ecr.aws/docker/library/python:3.12

COPY --from=build /dg-sat/bin/dg-sat /usr/bin
COPY --from=build /tmp/lshw.txt /usr/share/
COPY --from=build /tmp/ostree_repo /usr/share/ostree_repo

RUN apt update && apt install -y ostree && rm -rf /var/lib/apt/lists/*

RUN pip install locust==2.43.4
