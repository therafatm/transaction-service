package dbutils

import (
	"database/sql"
	"io/ioutil"
	"log"
	"net/http"
	"os"

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

	return string("http:quoteserve.seng:4444")
}

func QueryQuote(username string, stock string) (body []byte, err error) {
	URL := GetQuoteServerURL()
	res, err := http.Get(URL + "/api/getQuote/" + username + "/" + stock)

	if err != nil {
		utils.LogErr(err)
	} else {
		body, err = ioutil.ReadAll(res.Body)
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

func QueryUserStockTrigger(username string, stock string) (string, int, error) {
	var shares int

	query := "SELECT shares FROM triggers WHERE username=$1 AND symbol=$2"
	err := db.QueryRow(query, username, stock).Scan(&shares)

	if err != nil {
		if err == sql.ErrNoRows {
			log.Println("Trigger does not exist.")
		}
		utils.LogErr(err)
		return string(""), -1, err
	}

	return stock, shares, err
}

func QueryLastReservation(username string, orderType string) (string, int, float64, error) {
	var symbol string
	var shares int
	var faceValue float64

	query := "SELECT symbol, shares, face_value FROM reservations WHERE username=$1 and type=$2 ORDER BY (time) DESC LIMIT 1"
	err := db.QueryRow(query, username, orderType).Scan(&symbol, &shares, &faceValue)
	return symbol, shares, faceValue, err
}
