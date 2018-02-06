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

	"transaction_service/queries/models"

	"github.com/go-redis/redis"
)

func queryRedisKey(cache *redis.Client, queryStruct *models.StockQuote) (err error) {
	key := fmt.Sprintf("%s:%s", queryStruct.Username, queryStruct.Symbol)

	if queryStruct.Qtype == models.CacheGet {
		val, err := cache.Get(key).Result()
		if err != nil {
			queryStruct.Value = val
		}
	} else {
		_, err = cache.Set(key, queryStruct.Value, time.Minute*1).Result()
	}
	return
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

func QueryQuoteTCP(cache *redis.Client, username string, stock string) (queryString string, err error) {
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

func QueryQuotePrice(cache *redis.Client, username string, symbol string, trans string) (quote int, err error) {
	var body string
	var setCache = true

	queryStruct := &models.StockQuote{Username: username, Symbol: symbol, Qtype: models.CacheGet}
	err = queryRedisKey(cache, queryStruct)
	if err == nil {
		body = queryStruct.Value
		setCache = false
	} else {
		_, exist := os.LookupEnv("PROD")
		if exist {
			body, err = QueryQuoteTCP(cache, username, symbol)
		} else {
			body, err = QueryQuoteHTTP(cache, username, symbol)
			fmt.Printf(body)
		}
		if err != nil {
			return
		}
	}

	split := strings.Split(body, ",")
	priceStr := strings.Replace(split[0], ".", "", 1)
	quote, err = strconv.Atoi(priceStr)
	if err != nil {
		return
	}

	if setCache {
		queryStruct.Qtype = models.CacheSet
		queryStruct.Value = priceStr
		err = queryRedisKey(cache, queryStruct)
		if err != nil {
			log.Println(err.Error())
		}
	}

	//logger.LogQuoteServ(username, split[0], split[1], split[3], split[4], trans)
	return
}
