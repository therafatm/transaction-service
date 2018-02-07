FROM golang:latest

EXPOSE 8888

COPY common /go/src/common
COPY transaction_service /go/src/transaction_service
WORKDIR /go/src/transaction_service


RUN apt-get update -y && apt-get install -y libxml2-dev

ENV WAITFORIT_VERSION="v2.2.0"
RUN curl -o /usr/local/bin/waitforit -sSL https://github.com/maxcnunes/waitforit/releases/download/$WAITFORIT_VERSION/waitforit-linux_amd64 && \
    chmod +x /usr/local/bin/waitforit


RUN go get github.com/pilu/fresh
RUN go get ./...



