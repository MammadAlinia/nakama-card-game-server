
FROM heroiclabs/nakama-pluginbuilder:3.41.0 AS builder

ENV GO111MODULE=on
ENV CGO_ENABLED=1

WORKDIR /backend

COPY go.mod ./
COPY *.go ./

RUN go mod tidy

RUN go build \
    --trimpath \
    --buildmode=plugin \
    -o ./healthcheck.so .

FROM heroiclabs/nakama:3.41.0

COPY --from=builder \
    /backend/healthcheck.so \
    /nakama/data/modules/healthcheck.so
