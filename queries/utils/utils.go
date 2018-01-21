package dbutils

import (
	"database/sql"
	"io/ioutil"
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
	return
}

func QueryUserStock(username string, symbol string) (string, int, error) {
	var sid string
	var shares int
	query := "SELECT sid, shares FROM stocks WHERE username = $1 AND symbol = $2"
	err := db.QueryRow(query, username, symbol).Scan(&sid, &shares)
	return sid, shares, err
}
