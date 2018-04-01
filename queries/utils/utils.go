	package dbutils

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"common/logging"
	"common/models"

	"github.com/go-redis/redis"
)

func getUnixTimestamp() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

func queryRedisKey(cache *redis.Client, queryStruct *models.StockQuote) error {
	key := fmt.Sprintf("%s", queryStruct.Symbol)
	var err error

	if queryStruct.Qtype == models.CacheGet {
		val, err := cache.Get(key).Result()
		if err == redis.Nil {
			err = errors.New("Key does not exist")
			return err
		} else if err == nil {
			queryStruct.Value = val
		}
	} else {
		_, err = cache.Set(key, queryStruct.Value, time.Second*50).Result()
	}

	return err
}

func QueryQuoteHTTP(cache *redis.Client, username string, stock string) (queryString string, err error) {
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

func QueryQuoteTCP(cache *redis.Client, username string, stock string) (string, error) {

	port := os.Getenv("QUOTE_SERVER_PORT")
	host := os.Getenv("QUOTE_SERVER_HOST")
	addr := strings.Join([]string{host, port}, ":")
	readTimeoutBase := time.Millisecond * 300
	backoff := time.Millisecond * 0
	maxAttempts := 9
	msg := stock + "," + username + "\n"
	var err error

	respBuf := make([]byte, 2048)
	attempts := 1

	for {
		quoteServerConn, err := net.DialTimeout("tcp", addr, readTimeoutBase)
		if err != nil {
			return "", err
		}

		quoteServerConn.SetWriteDeadline(time.Now().Add(readTimeoutBase))
		quoteServerConn.Write([]byte(msg))

		timeout := readTimeoutBase + backoff
		quoteServerConn.SetReadDeadline(time.Now().Add(timeout))

		_, err = quoteServerConn.Read(respBuf)
		quoteServerConn.Close()

		if err == nil {
			break
		}

		if attempts > maxAttempts {
			return "Quoteserver max attempts reached.", errors.New("Quoteserver max attempts for response")
		}

		// check for a timeout
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			// backoff linearly and try again for a quote
			log.Println("Attempt %d timeout. Waiting for %d ms", attempts, timeout)
		} else {
			return "Failed to read from quoteserver", errors.New("Failed to read from quoteserve")
		}
		attempts++
	}

	// clean up the unused space in the buffer
	respBuf = bytes.Trim(respBuf, "\x00")
	queryString := bytes.NewBuffer(respBuf).String()
	queryString = strings.TrimSpace(queryString)
	return queryString, err
}

func QueryQuotePrice(cache *redis.Client, logger logging.Logger, username string, symbol string, trans string) (quote int, err error) {
	var body string

	queryStruct := &models.StockQuote{Username: username, Symbol: symbol, Qtype: models.CacheGet, CrytpoKey: "", QuoteTimestamp: ""}
	err = queryRedisKey(cache, queryStruct)

	if err == nil {
		// cache hit
		quote, err = strconv.Atoi(queryStruct.Value)
		fmt.Println("Cache hit!")
		return
	}

	prod, _ := os.LookupEnv("PROD")
	if prod == "true" {
		body, err = QueryQuoteTCP(cache, username, symbol)
	} else {
		body, err = QueryQuoteHTTP(cache, username, symbol)
		fmt.Println("Printing body")
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

	// set cache
	queryStruct.Qtype = models.CacheSet
	queryStruct.Value = priceStr
	err = queryRedisKey(cache, queryStruct)
	if err != nil {
		log.Println(err.Error())
	}

	queryStruct.QuoteTimestamp = split[3]
	queryStruct.CrytpoKey = split[4]

	logger.LogQuoteServ(queryStruct, trans)
	fmt.Println(queryStruct)
	return
}
