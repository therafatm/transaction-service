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

func AddReservation(username string, stock string, orderType string, shares int, faceValue float64) (res sql.Result, err error) {
	// time in seconds
	time := time.Now().Unix()
	query := "INSERT INTO reservations(username, symbol, type, shares, face_value, time) VALUES($1,$2,$3,$4,$5,$6)"
	res, err = db.Exec(query, username, stock, orderType, shares, faceValue, time)
	return
}

func UpdateUserStock(tx *sql.Tx, username string, symbol string, shares int, orderType string, channel chan error) (err error) {
	_, currentShares, err := dbutils.QueryUserStock(username, symbol)

	if err != nil {
		utils.LogErr(err)
		if err == sql.ErrNoRows {
			query := "INSERT INTO stocks(username,symbol,shares) VALUES($1,$2,$3)"
			_, err = tx.Exec(query, username, symbol, shares)
			return
		}
		utils.LogErr(err)
		return
	}

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
	} else {
		_, err = tx.Exec(query, balance, username)
	}

	if channel != nil {
		channel <- err
	}

	if err != nil {
		utils.LogErr(err)
	}
	return
}

func RemoveReservation(tx *sql.Tx, username string, stock string, reservationType string, shares int, faceValue float64, channel chan error) (err error) {
	query := "DELETE FROM reservations WHERE username=$1 AND symbol=$2 AND shares=$3 AND face_value=$4 AND type=$5"
	if tx == nil {
		_, err = db.Exec(query, username, stock, shares, faceValue, reservationType)
	} else {
		_, err = tx.Exec(query, username, stock, shares, faceValue, reservationType)
	}

	if channel != nil {
		channel <- err
	}

	if err != nil {
		utils.LogErr(err)
	}
	return
}

func RemoveOrder(username string, stock string, reservationType string, shares int, faceValue float64) {
	time.Sleep(60 * time.Second)
	err := RemoveReservation(nil, username, stock, reservationType, shares, faceValue, nil)
	if err != nil {
		log.Println("Error removing reservation due to timeout.")
		utils.LogErr(err)
	}
}

func SetUserOrderTypeAmount(tx *sql.Tx, username string, stock string, orderType string, amount float64, channel chan error) (err error) {

	_, _, _, err = dbutils.QueryUserStockTrigger(username, stock, orderType)

	if err != nil {
		query := "INSERT INTO triggers(username, symbol, type, amount) VALUES($1,$2,$3,$4)"
		if tx != nil {
			_, err = tx.Exec(query, username, stock, orderType, amount)
		} else {
			_, err = db.Exec(query, username, stock, orderType, amount)
		}
	} else {
		// Already have a trigger set for this stock
		// Cancel first, then apply for new one
		log.Println("Trigger of type " + orderType + " exists for stock: " + stock)
		s := fmt.Sprintf("Trigger exists of type %s for stock: %s\nCancel current trigger and request again.", orderType, stock)
		err = errors.New(s)
		return
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

func UpdateUserStockTriggerPrice(username string, stock string, triggerPrice string) (err error) {

	query := "UPDATE triggers SET triggerPrice=$1 WHERE username=$2 AND symbol=$3"
	_, err = db.Exec(query, triggerPrice, username, stock)

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

func CommitTransaction(username string, orderType string) []byte {
	var symbol string
	var shares int
	var faceValue float64
	var err error

	symbol, shares, faceValue, err = dbutils.QueryLastReservation(username, orderType)

	if err != nil {
		utils.LogErr(err)
		return []byte("Error retrieving reservation.")
	}

	amount := float64(shares) * faceValue
	queryResults := make(chan error)

	tx, err := db.Begin()

	go UpdateUserStock(tx, username, symbol, shares, orderType, queryResults)
	go UpdateUserMoney(tx, username, amount, orderType, queryResults)
	go RemoveReservation(tx, username, symbol, orderType, shares, faceValue, queryResults)

	err1, err2, err3 := <-queryResults, <-queryResults, <-queryResults
	if err != nil || err1 != nil || err2 != nil || err3 != nil {
		tx.Rollback()
		return []byte("Error querying within transaction.")
	}

	err = tx.Commit()
	if err != nil {
		utils.LogErr(err)
		tx.Rollback()
		return []byte("Error committing transaction.")
	}

	return []byte("Sucessfully comitted transaction.")
}

func CommitSetBuyAmountTx(username string, symbol string, orderType string, buyAmount float64) []byte {

	queryResults := make(chan error)
	tx, err := db.Begin()

	go UpdateUserMoney(tx, username, buyAmount, orderType, queryResults)
	go SetUserOrderTypeAmount(tx, username, symbol, orderType, buyAmount, queryResults)

	err1, err2 := <-queryResults, <-queryResults

	if err != nil || err1 != nil || err2 != nil {
		tx.Rollback()
		return []byte("Error querying within transaction.")
	}

	err = tx.Commit()
	if err != nil {
		utils.LogErr(err)
		tx.Rollback()
		return []byte("Error committing transaction.")
	}

	m := string("Sucessfully comitted SET " + orderType + " transaction.")
	return []byte(m)
}

func SetBuyTrigger(username string, symbol string, triggerPrice string) []byte {

	err := UpdateUserStockTriggerPrice(username, symbol, triggerPrice)
	if err != nil {
		return []byte("Failed to update trigger.")
	}

	return []byte("Sucessfully comitted SET BUY TRIGGER transaction.")
}

func SetSellTrigger(username string, symbol string, totalValue float64, triggerPrice float64) []byte {

	orderType := "sell"
	shares := int(totalValue / triggerPrice)
	sharesStr := strconv.Itoa(shares)
	totalValueStr := fmt.Sprintf("%f", totalValue)
	queryResults := make(chan error)
	tx, err := db.Begin()

	go UpdateUserStock(tx, username, totalValueStr, shares, orderType, queryResults)
	go UpdateUserStockTriggerShares(tx, username, symbol, sharesStr)

	err1, err2 := <-queryResults, <-queryResults

	if err != nil || err1 != nil || err2 != nil {
		tx.Rollback()
		return []byte("Error querying within transaction.")
	}

	err = tx.Commit()
	if err != nil {
		utils.LogErr(err)
		tx.Rollback()
		return []byte("Error committing transaction.")
	}

	return []byte("Sucessfully comitted SET SELL transaction.")
}

func CancelSetTrigger(username string, symbol string, orderType string) []byte {

	_, shares, totalValue, err := dbutils.QueryUserStockTrigger(username, symbol, orderType)
	isSell := strings.Compare(orderType, "sell") == 0
	//adds stock back

	queryResults := make(chan error)
	tx, err := db.Begin()

	if isSell {
		orderType := "buy"
		//adds stock back
		go UpdateUserStock(tx, username, symbol, int(shares), orderType, queryResults)
	} else {
		orderType := "buy"
		//adds money back
		go UpdateUserMoney(tx, username, totalValue, orderType, queryResults)
	}

	go RemoveUserStockTrigger(tx, username, symbol, orderType, queryResults)

	err1, err2 := <-queryResults, <-queryResults

	if err != nil || err1 != nil || err2 != nil {
		tx.Rollback()
		return []byte("Error querying within transaction.")
	}

	err = tx.Commit()
	if err != nil {
		utils.LogErr(err)
		tx.Rollback()
		return []byte("Error committing transaction.")
	}

	return []byte("Sucessfully comitted CANCEL SET SELL TRIGGER transaction.")
}

func ExecuteTrigger(username string, symbol string, shares string, totalValue float64, triggerValue float64, orderType string) []byte {

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

	go UpdateUserStock(tx, username, symbol, sharesInt, orderType, queryResults)
	go RemoveUserStockTrigger(tx, username, symbol, orderType, queryResults)
	if isSellOrder {
		go UpdateUserMoney(tx, username, totalValue, orderType, queryResults)
	}

	err1, err2 := <-queryResults, <-queryResults
	if isSellOrder {
		err3 = <-queryResults
	}

	if err != nil || err1 != nil || err2 != nil || err3 != nil {
		tx.Rollback()
		return []byte("Error querying within transaction.")
	}

	err = tx.Commit()
	if err != nil {
		utils.LogErr(err)
		tx.Rollback()
		return []byte("Error committing transaction.")
	}

	return []byte("Sucessfully executed SET BUY trigger.")
}
