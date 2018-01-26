FROM golang:latest

EXPOSE 8888

COPY . /go/src/transaction_service
WORKDIR /go/src/transaction_service

RUN apt-get update && apt-get install nano
RUN go get github.com/pilu/fresh
RUN go get ./...


ENV WAITFORIT_VERSION="v2.2.0"
RUN curl -o /usr/local/bin/waitforit -sSL https://github.com/maxcnunes/waitforit/releases/download/$WAITFORIT_VERSION/waitforit-linux_amd64 && \
    chmod +x /usr/local/bin/waitforit


