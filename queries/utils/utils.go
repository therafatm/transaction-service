package dbutils

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"../../utils"
)

var db *sql.DB

// var quoteServerPort = os.Getenv("QUOTE_SERVER_PORT")
var quoteServerPort = "8000"

func SetUtilsDB(database *sql.DB) {
	db = database
}

func GetQuoteServerURL() string {
	if os.Getenv("GO_ENV") == "dev" {
		return string("http://localhost:" + quoteServerPort)
	}

	return string("http://quoteserver:8000")
}

func QueryQuote(username string, stock string) (body []byte, err error) {
	URL := GetQuoteServerURL()
	log.Println(URL)
	res, err := http.Get(URL + "/api/getQuote/" + username + "/" + stock)

	if err != nil {
		utils.LogErr(err)
	} else {
		body, err = ioutil.ReadAll(res.Body)
		log.Println(string(body))
	}

	return
}

func QueryUser(username string) (uid string, balance float64, err error) {
	query := "SELECT uid, money FROM users WHERE username = $1"
	err = db.QueryRow(query, username).Scan(&uid, &balance)
	if err != nil {
		utils.LogErr(err)
	}
	return
}

func QueryUserStock(username string, symbol string) (string, int, error) {
	var sid string
	var shares int
	query := "SELECT sid, shares FROM stocks WHERE username = $1 AND symbol = $2"
	err := db.QueryRow(query, username, symbol).Scan(&sid, &shares)
	return sid, shares, err
}

func QueryUserStockTrigger(username string, stock string, orderType string) (string, int64, float64, error) {
	var shares sql.NullInt64
	var totalAmount sql.NullFloat64

	query := "SELECT shares, amount FROM triggers WHERE username=$1 AND symbol=$2 AND type=$3"
	err := db.QueryRow(query, username, stock, orderType).Scan(&shares, &totalAmount)

	if err != nil {
		if err == sql.ErrNoRows {
			log.Println("Trigger does not exist.")
		}
		utils.LogErr(err)
		return string(""), -1, -1, err
	}

	return stock, shares.Int64, totalAmount.Float64, err
}

func QueryAndExecuteCurrentTriggers() {

	query := "SELECT username, symbol, type, shares, amount,triggerprice FROM triggers WHERE triggerprice IS NOT NULL"
	rows, err := db.Query(query)

	if err != nil {
		utils.LogErr(err)
	}

	defer rows.Close()
	var username string
	var symbol string
	var orderType string
	var shares sql.NullInt64
	var amount sql.NullFloat64
	var triggerValue sql.NullFloat64

	for rows.Next() {
		err := rows.Scan(&username, &symbol, &orderType, &shares, &amount, &triggerValue)
		if err != nil {
			utils.LogErr(err)
		}

		isSell := strings.Compare(orderType, "sell") == 0
		if (isSell && shares.Int64 > 0) || (!isSell && triggerValue.Float64 > 0) {
			log.Println(username)
			log.Println(symbol)
			quoteStr, err := QueryQuote(username, symbol)
			if err == nil {
				quote, _ := strconv.ParseFloat(strings.Split(string(quoteStr), ",")[0], 64)
				if quote <= triggerValue.Float64 {
					url := fmt.Sprintf("http://localhost:8888/api/executeTrigger/%s/%s/%d/%f/%f/%s", username, symbol, shares.Int64, amount.Float64, triggerValue.Float64, orderType)
					log.Println(url)
					go http.Get(url)
				}
			} else {
				utils.LogErr(err)
			}
		}
	}

	return
}

func QueryLastReservation(username string, orderType string) (string, int, float64, error) {
	var symbol string
	var shares int
	var amount float64

	query := "SELECT symbol, shares, amount FROM reservations WHERE username=$1 and type=$2 ORDER BY (time) DESC LIMIT 1"
	err := db.QueryRow(query, username, orderType).Scan(&symbol, &shares, &amount)
	return symbol, shares, amount, err
}
