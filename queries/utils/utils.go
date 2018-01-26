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

	//"transaction_service/utils"
	"transaction_service/queries/models"
)

var db *sql.DB

func SetUtilsDB(database *sql.DB) {
	db = database
}

func ScanTrigger(row *sql.Row) (trig models.Trigger, err error){
	err = row.Scan(&trig.ID, &trig.Username, &trig.Symbol, &trig.Order, &trig.Amount, &trig.TriggerPrice, &trig.Executable, &trig.Time)
	return
}

func ScanTriggerRows(rows *sql.Rows) (trig models.Trigger, err error){
	err = rows.Scan(&trig.ID, &trig.Username, &trig.Symbol, &trig.Order, &trig.Amount, &trig.TriggerPrice, &trig.Executable, &trig.Time)
	return
}

func GetQuoteServerURL() string {
    port := os.Getenv("QUOTE_SERVER_PORT")
    host := os.Getenv("QUOTE_SERVER_HOST")
    url := fmt.Sprintf("http://%s:%s", host, port)
    return string(url)
}

func QueryQuote(username string, stock string) (body []byte, err error) {
	URL := GetQuoteServerURL()
	log.Println(URL)
	res, err := http.Get(URL + "/api/getQuote/" + username + "/" + stock)

	if err != nil {
		return
	} else {
		body, err = ioutil.ReadAll(res.Body)
		log.Println(string(body))
	}
	return
}

func QueryQuotePrice(username string, symbol string) (quote int, err error) {
	body, err := QueryQuote(username, symbol)
	if err != nil {
		return
	}
	priceStr :=  strings.Replace(strings.Split(string(body), ",")[0], ".", "", 1)
	quote, err = strconv.Atoi(priceStr)
	if err != nil {
		return
	}
	return
}

func QueryUserAvailableBalance(username string) ( balance int, err error) {
	query := `SELECT (SELECT money FROM USERS WHERE username = $1) as available_balance;`
	err = db.QueryRow(query, username).Scan(&balance)
	return
}

func QueryUserAvailableShares(username string, symbol string) (shares int, err error) {
	query := `SELECT (SELECT COALESCE(SUM(shares), 0) FROM Stocks WHERE username = $1 and symbol = $2)`
	err = db.QueryRow(query, username, symbol).Scan(&shares)
	return 
}

func QueryUser(username string) (user models.User, err error) {
	query := "SELECT uid, username, money FROM users WHERE username = $1"
	err = db.QueryRow(query, username).Scan(&user.ID, &user.Username, &user.Money)
	return
}

func QueryUserStock(username string, symbol string) (stock models.Stock, err error) {
	query := "SELECT sid, username, symbol, shares FROM stocks WHERE username = $1 AND symbol = $2"
	err = db.QueryRow(query, username, symbol).Scan(&stock.ID, &stock.Username, &stock.Symbol, &stock.Shares)
	return 
}

func QueryStockTrigger(tid int64) (trig models.Trigger, err error) {
	query := "SELECT tid, username, symbol, type, amount, trigger_price, executable, time FROM triggers WHERE tid = $1"
	trig, err = ScanTrigger(db.QueryRow(query, tid))
	return 
}

func QueryUserTrigger(username string, symbol string, orderType models.OrderType) (trig models.Trigger, err error) {
	query := "SELECT tid, username, symbol, type, amount, trigger_price, executable, time FROM triggers WHERE username = $1 AND symbol=$2 AND type=$3"
	trig, err = ScanTrigger(db.QueryRow(query, username, symbol, orderType))
	return 
}

func QueryReservation(rid int64) (res models.Reservation, err error) {
	query := "SELECT rid, username, symbol, shares, amount, type, time FROM reservations WHERE rid=$1"
	err = db.QueryRow(query, rid).Scan(&res.ID, &res.Username, &res.Symbol, &res.Shares, &res.Amount, &res.Order, &res.Time)
	return
}

func QueryLastReservation(username string, resType models.OrderType) (res models.Reservation, err error) {
	query := "SELECT rid, username, symbol, shares, amount, type, time FROM reservations WHERE username=$1 and type=$2 ORDER BY (time) DESC, rid DESC LIMIT 1"
	err = db.QueryRow(query, username, resType).Scan(&res.ID, &res.Username, &res.Symbol, &res.Shares, &res.Amount, &res.Order, &res.Time)
	return
}
