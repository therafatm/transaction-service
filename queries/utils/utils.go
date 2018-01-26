package dbutils

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	//"transaction_service/utils"
	logger "transaction_service/logger"
	"transaction_service/queries/models"
	"transaction_service/utils"
)

var db *sql.DB

func SetUtilsDB(database *sql.DB) {
	db = database
}

func ScanTrigger(row *sql.Row) (trig models.Trigger, err error) {
	err = row.Scan(&trig.ID, &trig.Username, &trig.Symbol, &trig.Order, &trig.Amount, &trig.TriggerPrice, &trig.Executable, &trig.Time)
	return
}

func ScanTriggerRows(rows *sql.Rows) (trig models.Trigger, err error) {
	err = rows.Scan(&trig.ID, &trig.Username, &trig.Symbol, &trig.Order, &trig.Amount, &trig.TriggerPrice, &trig.Executable, &trig.Time)
	return
}

func GetQuoteServerURL() string {
	port := os.Getenv("QUOTE_SERVER_PORT")
	host := os.Getenv("QUOTE_SERVER_HOST")
	url := fmt.Sprintf("http://%s:%s", host, port)
	return string(url)
}

func QueryQuote(username string, stock string) ([]byte, error) {

	var body = make([]byte, 1024)
	var err error

	env := strings.Compare(os.Getenv("ENV"), "prod") == 0

	if env == true {
		ip := "192.168.1.152"
		port := "4445"
		addr := strings.Join([]string{ip, port}, ":")
		conn, err := net.DialTimeout("tcp", addr, time.Second*10)
		if err != nil {
			return body, err
		}
		defer conn.Close()

		msg := stock + "," + username + "\n"
		conn.Write([]byte(msg))

		_, err = conn.Read(body)
		log.Println(string(body))
	} else {
		URL := GetQuoteServerURL()
		res, err := http.Get(URL + "/api/getQuote/" + username + "/" + stock + "/0")
		if err != nil {
			utils.LogErr(err)
		} else {
			body, err = ioutil.ReadAll(res.Body)
			log.Println(string(body))
		}
	}

	return body, err
}

func QueryQuotePrice(username string, symbol string) (quote int, err error) {
	body, err := QueryQuote(username, symbol)
	if err != nil {
		return
	}

	split := strings.Split(string(body), ",")

	priceStr := strings.Replace(split[0], ".", "", 1)
	quote, err = strconv.Atoi(priceStr)
	if err != nil {
		return
	}

	logger.LogQuoteServ(username, split[0], split[1], split[2], split[3])
	return
}

func QueryUserAvailableBalance(username string) (balance int, err error) {
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
