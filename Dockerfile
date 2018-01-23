FROM golang:latest

EXPOSE 8888

COPY . /go/src/transaction-service
WORKDIR /go/src/transaction-service

RUN go get github.com/pilu/fresh
RUN go get ./...

ENTRYPOINT ["fresh", "app.go"]

