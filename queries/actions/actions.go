package dbactions

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"../../utils"
	"../utils"
)

var db *sql.DB

func SetActionsDB(database *sql.DB) {
	db = database
}

func InsertUser(username string, money string) (err error) {
	//add new user
	query := "INSERT INTO users(username, money) VALUES($1,$2)"
	_, err = db.Exec(query, username, money)
	if err != nil {
		log.Fatal(err)
	}
	return
}

func UpdateUser(username string, money string) (err error) {
	query := "UPDATE users SET money = $1 WHERE username = $2"
	_, err = db.Exec(query, money, username)
	if err != nil {
		utils.LogErr(err)
	}
	return
}

func AddReservation(tx *sql.Tx, username string, stock string, orderType string, shares int, amount float64, queryResults chan error) (err error) {
	// time in seconds
	time := time.Now().Unix()
	query := "INSERT INTO reservations(username, symbol, type, shares, amount, time) VALUES($1,$2,$3,$4,$5,$6)"

	if tx == nil {
		_, err = db.Exec(query, username, stock, orderType, shares, amount, time)
	} else {
		_, err = db.Exec(query, username, stock, orderType, shares, amount, time)
	}

	if queryResults != nil {
		queryResults <- err
	}

	return
}

func UpdateUserStock(tx *sql.Tx, username string, symbol string, shares int, orderType string, channel chan error) (err error) {

	_, currentShares, err := dbutils.QueryUserStock(username, symbol)

	if err != nil {
		utils.LogErr(err)
		if err == sql.ErrNoRows {
			query := "INSERT INTO stocks(username,symbol,shares) VALUES($1,$2,$3)"
			_, err = tx.Exec(query, username, symbol, shares)
			log.Println("Finished updating stock")
			utils.LogErr(err)
			return
		}
		utils.LogErr(err)
		return
	}

	log.Println(currentShares)
	log.Println(shares)

	if strings.Compare(orderType, "buy") == 0 {
		currentShares += shares
	} else {
		currentShares -= shares
	}

	query := "UPDATE stocks SET shares=$1 WHERE username=$2 AND symbol=$3"
	_, err = tx.Exec(query, currentShares, username, symbol)

	if channel != nil {
		channel <- err
	}

	if err != nil {
		utils.LogErr(err)
	}

	log.Println("Finished updating stock")
	return
}

func UpdateUserMoney(tx *sql.Tx, username string, money float64, orderType string, channel chan error) (err error) {
	_, balance, err := dbutils.QueryUser(username)

	if err != nil {
		utils.LogErr(err)
		return
	}

	if strings.Compare(orderType, "buy") == 0 {
		balance -= money
	} else {
		balance += money
	}

	query := "UPDATE users SET money=$1 WHERE username=$2"
	if tx == nil {
		_, err = db.Exec(query, balance, username)
		log.Println("hey now")
	} else {
		_, err = tx.Exec(query, balance, username)
		log.Println("brown cow")
	}

	if channel != nil {
		channel <- err
	}

	if err != nil {
		utils.LogErr(err)
	}

	log.Println("Finished updating user money.")
	return
}

func RemoveReservation(tx *sql.Tx, username string, stock string, orderType string, channel chan error) (err error) {
	query := "DELETE FROM reservations WHERE username=$1 AND symbol=$2 AND type=$3"
	if tx == nil {
		_, err = db.Exec(query, username, stock, orderType)
	} else {
		_, err = tx.Exec(query, username, stock, orderType)
	}

	if channel != nil {
		channel <- err
	}

	if err != nil {
		utils.LogErr(err)
	}

	log.Println("Finished updating reservations")
	return
}

func RemoveOrder(username string, stock string, orderType string, timeout time.Duration) {
	time.Sleep(timeout * time.Second)

	err := RemoveReservation(nil, username, stock, orderType, nil)
	if err != nil {
		log.Println("Error removing reservation due to timeout.")
		utils.LogErr(err)
	}
}

func RemoveLastOrderTypeReservation(username string, orderType string) (err error) {

	query := `DELETE FROM reservations WHERE rid IN ( 
				SELECT rid FROM reservations WHERE username=$1 AND type=$2 ORDER BY time DESC LIMIT(1) 
			 )`

	_, err = db.Exec(query, username, orderType)

	if err != nil {
		utils.LogErr(err)
	}

	log.Println("Finished updating reservations")
	return
}

func ExecuteSetBuyAmount(username string, symbol string, orderType string, buyAmount float64) (err error) {

	tx, err := db.Begin()

	err = SetUserOrderTypeAmount(tx, username, symbol, orderType, buyAmount, nil)
	if err != nil {
		utils.LogErr(err)
		return
	}

	err = UpdateUserMoney(tx, username, buyAmount, orderType, nil)
	if err != nil {
		utils.LogErr(err)
	}

	err = tx.Commit()
	if err != nil {
		utils.LogErr(err)
		tx.Rollback()
		return
	}

	return
}

func SetUserOrderTypeAmount(tx *sql.Tx, username string, stock string, orderType string, amount float64, channel chan error) (err error) {

	query := "INSERT INTO triggers(username, symbol, type, amount) VALUES($1,$2,$3,$4)"
	if tx != nil {
		_, err = tx.Exec(query, username, stock, orderType, amount)
	} else {
		_, err = db.Exec(query, username, stock, orderType, amount)
	}

	if err != nil {
		utils.LogErr(err)
	}

	if channel != nil {
		channel <- err
	}
	return
}

func RemoveUserStockTrigger(tx *sql.Tx, username string, stock string, orderType string, channel chan error) (err error) {

	query := "DELETE FROM triggers WHERE username=$1 AND symbol=$2 AND type=$3"
	if tx != nil {
		_, err = tx.Exec(query, username, stock, orderType)
	} else {
		_, err = db.Exec(query, username, stock, orderType)
	}

	if err != nil {
		utils.LogErr(err)
	}

	if channel != nil {
		channel <- err
	}
	return
}

func UpdateUserStockTriggerPrice(username string, stock string, orderType string, triggerPrice string) (err error) {

	query := "UPDATE triggers SET trigger_price=$1 WHERE username=$2 AND symbol=$3 AND type=$4"
	_, err = db.Exec(query, triggerPrice, username, stock, orderType)

	if err != nil {
		utils.LogErr(err)
	}

	return
}

func UpdateUserStockTriggerShares(tx *sql.Tx, username string, stock string, shares string) (err error) {

	query := "UPDATE triggers SET shares=$1 WHERE username=$2 AND symbol=$3"
	if tx == nil {
		_, err = db.Exec(query, shares, username, stock)
	} else {
		_, err = tx.Exec(query, shares, username, stock)
	}

	if err != nil {
		utils.LogErr(err)
	}

	return
}

func CommitBuySellTransaction(username string, orderType string) (err error) {
	var symbol string
	var shares int
	var amount float64

	symbol, shares, amount, err = dbutils.QueryLastReservation(username, orderType)

	if err != nil {
		utils.LogErr(err)
		return
	}

	tx, err := db.Begin()

	err1 := UpdateUserStock(tx, username, symbol, shares, orderType, nil)
	err2 := UpdateUserMoney(tx, username, amount, orderType, nil)
	err3 := RemoveReservation(tx, username, symbol, orderType, nil)

	if err != nil || err1 != nil || err2 != nil || err3 != nil {
		tx.Rollback()
		err = errors.New("Error querying within transaction.\n")
		return
	}

	err = tx.Commit()
	if err != nil {
		utils.LogErr(err)
		tx.Rollback()
		return
	}

	return
}

func BuyOrderTx(username string, symbol string, orderType string, buyUnits int, buyAmount float64) (err error) {

	tx, err := db.Begin()

	err1 := AddReservation(tx, username, symbol, orderType, buyUnits, buyAmount, nil)

	if err != nil || err1 != nil {
		tx.Rollback()
		err = errors.New("Error querying within transaction.\n")
		return
	}

	err = tx.Commit()
	if err != nil {
		utils.LogErr(err)
		tx.Rollback()
		return
	}

	return
}

func SetBuyTrigger(username string, symbol string, orderType string, triggerPrice string) (err error) {

	err = UpdateUserStockTriggerPrice(username, symbol, orderType, triggerPrice)
	if err != nil {
		utils.LogErr(err)
		return
	}

	return
}

func SetSellTrigger(username string, symbol string, totalValue float64, triggerPrice float64) (err error) {

	orderType := "sell"
	shares := int(totalValue / triggerPrice)
	sharesStr := strconv.Itoa(shares)
	totalValueStr := fmt.Sprintf("%f", totalValue)

	tx, err := db.Begin()

	err1 := UpdateUserStock(tx, username, totalValueStr, shares, orderType, nil)
	err2 := UpdateUserStockTriggerShares(tx, username, symbol, sharesStr)

	if err != nil || err1 != nil || err2 != nil {
		tx.Rollback()
		err = errors.New("error querying within transaction")
		utils.LogErr(err)
		return
	}

	err = tx.Commit()
	if err != nil {
		utils.LogErr(err)
		tx.Rollback()
		return
	}

	return
}

func CancelSetTrigger(username string, symbol string, orderType string) (err error) {

	_, shares, totalValue, err := dbutils.QueryUserStockTrigger(username, symbol, orderType)
	if err != nil {
		// DB error or no trigger exists
		return
	}

	isSell := strings.Compare(orderType, "sell") == 0
	var err1 error

	tx, err := db.Begin()

	if isSell {
		orderType := "buy"
		//adds stock back
		err1 = UpdateUserStock(tx, username, symbol, int(shares), orderType, nil)
	} else {
		orderType := "sell"
		//adds money back
		err1 = UpdateUserMoney(tx, username, totalValue, orderType, nil)
	}

	err2 := RemoveUserStockTrigger(tx, username, symbol, orderType, nil)

	if err != nil || err1 != nil || err2 != nil {
		tx.Rollback()
		err = errors.New("error querying within transaction")
		return
	}

	err = tx.Commit()
	if err != nil {
		utils.LogErr(err)
		tx.Rollback()
		return
	}

	return
}

func ExecuteTrigger(username string, symbol string, shares string, totalValue float64, triggerValue float64, orderType string) (err error) {

	var err3 error = nil
	var sharesInt int
	queryResults := make(chan error)
	isSellOrder := strings.Compare(orderType, "sell") > 0
	if !isSellOrder {
		sharesInt = int(totalValue / triggerValue)
	} else {
		sharesInt, _ = strconv.Atoi(shares)
	}

	tx, err := db.Begin()

	err1 := UpdateUserStock(tx, username, symbol, sharesInt, orderType, queryResults)
	err2 := RemoveUserStockTrigger(tx, username, symbol, orderType, queryResults)
	if isSellOrder {
		err3 = UpdateUserMoney(tx, username, totalValue, orderType, queryResults)
	}

	if err != nil || err1 != nil || err2 != nil || err3 != nil {
		tx.Rollback()
		err = errors.New("Error querying within transaction.")
		return
	}

	err = tx.Commit()
	if err != nil {
		utils.LogErr(err)
		tx.Rollback()
		return
	}

	return
}
