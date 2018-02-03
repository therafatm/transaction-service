package dbutils

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func QueryQuoteHTTP(username string, stock string) (queryString string, err error) {
	port := os.Getenv("QUOTE_SERVER_PORT")
	host := os.Getenv("QUOTE_SERVER_HOST")
	url := fmt.Sprintf("http://%s:%s", host, port)
	res, err := http.Get(url + "/api/getQuote/" + username + "/" + stock)
	if err != nil {
		return
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}
	queryString = string(body)
	log.Println(queryString)
	return
}

func QueryQuoteTCP(username string, stock string) (queryString string, err error) {
	port := os.Getenv("QUOTE_SERVER_PORT")
	host := os.Getenv("QUOTE_SERVER_HOST")
	addr := strings.Join([]string{host, port}, ":")
	conn, err := net.DialTimeout("tcp", addr, time.Second*10)
	if err != nil {
		return queryString, err
	}
	defer conn.Close()

	msg := stock + "," + username + "\n"
	conn.Write([]byte(msg))

	buff, err := ioutil.ReadAll(conn)

	queryString = strings.TrimSpace(string(buff))
	log.Println(queryString)
	return
}

func QueryQuotePrice(username string, symbol string, trans string) (quote int, err error) {
	var body string
	_, exist := os.LookupEnv("PROD")
	if exist {
		body, err = QueryQuoteTCP(username, symbol)
	} else {
		body, err = QueryQuoteHTTP(username, symbol)
		fmt.Printf(body)
	}
	if err != nil {
		return
	}

	split := strings.Split(body, ",")
	priceStr := strings.Replace(split[0], ".", "", 1)
	quote, err = strconv.Atoi(priceStr)
	if err != nil {
		return
	}

	//logger.LogQuoteServ(username, split[0], split[1], split[3], split[4], trans)
	return
}
