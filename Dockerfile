FROM golang:latest

EXPOSE 8888

WORKDIR /go/src/transaction_service
ADD . /go/src/transaction_service



RUN go get github.com/pilu/fresh
RUN go get ./...


#RUN go build app.go

ENTRYPOINT ["fresh", "app.go"]
