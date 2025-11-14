FROM golang:1.25-alpine3.22 AS builder

RUN apk update
RUN apk add  git
ARG VERSION
ARG COMMIT
WORKDIR /app

COPY . ./
RUN go mod download


RUN go build -o /app/twsla -ldflags "-s -w -X main.version=$VERSION -X main.commit=$COMMIT" .

FROM alpine

RUN apk add --update --no-cache tzdata
WORKDIR /datastore
COPY --from=builder /app/twsla /
ENTRYPOINT [ "/twsla" ]
