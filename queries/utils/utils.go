package dbutils

import (
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


type quoteResponse struct {
	var response string
	var err	error
}

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

func QueryQuoteTCP(cache *redis.Client, username string, stock string, port string, responseChannel chan quoteResponse) {
	// port := os.Getenv("QUOTE_SERVER_PORT")
	host := os.Getenv("QUOTE_SERVER_HOST")
	addr := strings.Join([]string{host, port}, ":")
	conn, err := net.DialTimeout("tcp", addr, time.Second*5)
	if err != nil {
		return queryString, err
	}
	defer conn.Close()

	msg := stock + "," + username + "\n"
	conn.Write([]byte(msg))

	buff, err := ioutil.ReadAll(conn)

	queryString = strings.TrimSpace(string(buff))
	log.Println(queryString)
	
	responseChannel <- quoteResponse{response: queryString, err: err}
	return
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

	responseChannel := make(chan quoteResponse)
	prod, _ := os.LookupEnv("PROD")

	if prod == "true" {
		port := os.Getenv("QUOTE_SERVER_PORT_0")
		go QueryQuoteTCP(cache, username, symbol, port, responseChannel)
		port := os.Getenv("QUOTE_SERVER_PORT_1")
		go QueryQuoteTCP(cache, username, symbol, port, responseChannel)
		port := os.Getenv("QUOTE_SERVER_PORT_2")
		go QueryQuoteTCP(cache, username, symbol, port, responseChannel)

		count := 3
		var res quoteResponse
		for count < 3 {
			res := <- responseChannel
			if res.err == nil {
				count++;
			} else {
				break;
			}
		}

		err := res.err
		body := res.response
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

	queryStruct.CrytpoKey = split[4]
	quoteTimestamp := int(getUnixTimestamp())
	queryStruct.QuoteTimestamp = strconv.Itoa(quoteTimestamp)

	logger.LogQuoteServ(queryStruct, trans)
	return
}
