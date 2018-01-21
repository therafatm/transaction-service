FROM golang:latest

EXPOSE 8888

COPY . /go/src/transaction_service
WORKDIR /go/src/transaction_service

RUN go get github.com/pilu/fresh
RUN go get ./...

ENTRYPOINT ["fresh", "app.go"]
