package dbactions

import (
	"database/sql"
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
		} else {
			utils.LogErr(err)
			return
		}
	}

	if strings.Compare(orderType, "buy") == 0 {
		currentShares += shares
	} else {
		currentShares -= shares
	}

	log.Println(currentShares)
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
	log.Println("Balance is ")
	log.Println(balance)
	log.Println("Money is ")
	log.Println(money)

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

func SetUserBuyAmount(tx *sql.Tx, username string, stock string, orderType string, amount float64, channel chan error) (err error) {

	_, _, err = dbutils.QueryUserStockTrigger(username, stock)

	if err != nil {
		query := "INSERT INTO triggers(username, symbol, type, amount) VALUES($1,$2,$3,$4)"
		if tx != nil {
			_, err = tx.Exec(query, username, stock, orderType, amount)
		} else {
			_, err = db.Exec(query, username, stock, orderType, amount)
		}
	} else {
		query := "UPDATE triggers SET amount = $1"
		if tx != nil {
			_, err = tx.Exec(query, amount)
		} else {
			_, err = db.Exec(query, amount)
		}
	}

	if err != nil {
		utils.LogErr(err)
	}

	if channel != nil {
		channel <- err
	}
	return
}

func RemoveUserStockTrigger(tx *sql.Tx, username string, stock string, channel chan error) (err error) {

	query := "DELETE FROM triggers WHERE username=$1 AND symbol=$2"
	if tx != nil {
		_, err = tx.Exec(query, username, stock)
	} else {
		_, err = db.Exec(query, username, stock)
	}

	if err != nil {
		utils.LogErr(err)
	}

	if channel != nil {
		channel <- err
	}
	return
}

func UpdateUserStockTrigger(username string, stock string, triggerPrice string) (err error) {

	query := "UPDATE triggers SET triggerPrice=$1 WHERE username=$2 AND symbol=$3"
	_, err = db.Exec(query, triggerPrice, username, stock)

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
	go SetUserBuyAmount(tx, username, symbol, orderType, buyAmount, queryResults)

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

	return []byte("Sucessfully comitted SET BUY transaction.")
}

func SetBuyTrigger(username string, symbol string, triggerPrice string) []byte {

	err := UpdateUserStockTrigger(username, symbol, triggerPrice)
	if err != nil {
		return []byte("Failed to update trigger.")
	}

	return []byte("Sucessfully comitted SET BUY TRIGGER transaction.")
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
	go RemoveUserStockTrigger(tx, username, symbol, queryResults)
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
