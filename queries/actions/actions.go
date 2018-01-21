package dbactions

import (
	"database/sql"
	"log"
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

func GetLastReservation(username string, orderType string) (string, int, float64, error) {
	var symbol string
	var shares int
	var faceValue float64

	query := "SELECT symbol, shares, face_value FROM reservations WHERE username=$1 and type=$2 ORDER BY (time) DESC LIMIT 1"
	err := db.QueryRow(query, username, orderType).Scan(&symbol, &shares, &faceValue)
	return symbol, shares, faceValue, err
}

func AddReservation(username string, stock string, orderType string, shares int, faceValue float64) (res sql.Result, err error) {
	// time in seconds
	time := time.Now().Unix()
	query := "INSERT INTO reservations(username, symbol, type, shares, face_value, time) VALUES($1,$2,$3,$4,$5,$6)"
	res, err = db.Exec(query, username, stock, orderType, shares, faceValue, time)
	return
}

func UpdateUserStock(tx *sql.Tx, username string, symbol string, shares int, orderType string) (err error) {
	_, currentShares, err := dbutils.QueryUserStock(username, symbol)

	if err != nil {
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

	query := "UPDATE stocks SET shares=$1 WHERE username=$2 AND symbol=$3"
	_, err = tx.Exec(query, currentShares, username, symbol)

	return
}

func UpdateUserMoney(tx *sql.Tx, username string, money float64, orderType string) (err error) {
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

	if err != nil {
		utils.LogErr(err)
		return
	}

	query := "UPDATE users SET money=$1 WHERE username=$2"
	_, err = tx.Exec(query, balance, username)

	return
}

func RemoveReservation(tx *sql.Tx, username string, stock string, reservationType string, shares int, faceValue float64) (err error) {
	query := "DELETE FROM reservations WHERE username=$1 AND symbol=$2 AND shares=$3 AND face_value=$4 AND type=$5"
	if tx == nil {
		_, err = db.Exec(query, username, stock, shares, faceValue, reservationType)
	} else {
		_, err = tx.Exec(query, username, stock, shares, faceValue, reservationType)
	}
	return
}

func RemoveOrder(username string, stock string, reservationType string, shares int, faceValue float64) {
	time.Sleep(60 * time.Second)
	err := RemoveReservation(nil, username, stock, reservationType, shares, faceValue)
	if err != nil {
		log.Println("Error removing reservation due to timeout.")
		utils.LogErr(err)
	}
}

func CommitTransaction(username string, orderType string) []byte {
	var symbol string
	var shares int
	var faceValue float64

	symbol, shares, faceValue, err := GetLastReservation(username, orderType)

	if err != nil {
		utils.LogErr(err)
		return []byte("Error retrieving reservation.")
	}

	amount := float64(shares) * faceValue

	tx, err := db.Begin()
	err = UpdateUserMoney(tx, username, amount, orderType)
	if err != nil {
		utils.LogErr(err)
		tx.Rollback()
		return []byte("Error updating user.")
	}

	err = UpdateUserStock(tx, username, symbol, shares, orderType)
	if err != nil {
		utils.LogErr(err)
		tx.Rollback()
		return []byte("Error updating user stock.")
	}

	err = RemoveReservation(tx, username, symbol, orderType, shares, faceValue)
	if err != nil {
		utils.LogErr(err)
		tx.Rollback()
		return []byte("Error updating reservation.")
	}

	err = tx.Commit()
	if err != nil {
		utils.LogErr(err)
		tx.Rollback()
		return []byte("Error committing transaction.")
	}

	return []byte("Sucessfully comitted transaction.")
}
