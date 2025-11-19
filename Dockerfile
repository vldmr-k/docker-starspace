FROM bitnami/minideb AS starspace

WORKDIR /opt/starspace

# Install dependencies
RUN apt-get update && apt-get install -y \
  git \
  build-essential \
  cmake \
  curl \
  zip \
  && rm -rf /var/lib/apt/lists/*

RUN cd /tmp && git clone https://github.com/facebookresearch/Starspace.git . && \
  curl -LO https://archives.boost.io/release/1.83.0/source/boost_1_83_0.tar.gz && \
  tar -xzf boost_1_83_0.tar.gz && \
  make -e BOOST_DIR=boost_1_83_0 && \
  make embed_doc -e BOOST_DIR=boost_1_83_0 &&  \
  mv starspace /opt/starspace/starspace && \
  mv embed_doc /opt/starspace/embed_doc

FROM golang:1.25 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./

FROM builder AS pre-prod

WORKDIR /app

#COPY --from=pre-prod /app/main /app/main
RUN go build  -o /app/main /app/main.go

FROM bitnami/minideb AS prod

WORKDIR /app 

COPY --from=pre-prod /app/main .
COPY --from=starspace /opt /opt

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=30s --start-period=10s --retries=5 CMD ["curl", "-f", "http://localhost:8000/health"]

CMD ["/app/main"]

FROM builder AS dev

COPY --from=starspace /opt/starspace /opt/starspace

# Install Air (hot reload)
RUN go install github.com/air-verse/air@latest

WORKDIR /app

COPY . .

CMD ["air"]
