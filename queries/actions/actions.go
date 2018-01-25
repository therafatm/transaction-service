package dbactions

import (
	"database/sql"
	"log"
	//"strconv"
	//"strings"
	"time"
	"fmt"

	"transaction_service/queries/utils"
	"transaction_service/queries/models"
	"transaction_service/utils"
)

var db *sql.DB

func SetActionsDB(database *sql.DB) {
	db = database
}

func ClearUsers() (err error) {
	query := "DELETE FROM Users"
	_, err = db.Exec(query)
	if err != nil {
		utils.LogErr(err)
	}
	return
}

func InsertUser(user models.User) (res sql.Result, err error) {
	//add new user
	query := "INSERT INTO users(username, money) VALUES($1,$2)"
	res, err = db.Exec(query, user.Username, user.Money)
	if err != nil {
		utils.LogErr(err)
	}
	return
}

func UpdateUser(user models.User) (res sql.Result, err error) {
	query := "UPDATE users SET money = $1 WHERE username = $2"
	money := fmt.Sprintf("%d", user.Money)
	res, err = db.Exec(query, money, user.Username)
	if err != nil {
		utils.LogErr(err)
	}
	return
}

func AddReservation(tx *sql.Tx, reserv models.Reservation) (rid int64, err error) {
	query := "INSERT INTO reservations(username, symbol, type, shares, amount, time) VALUES($1,$2,$3,$4,$5,$6) RETURNING rid"
	if tx == nil {
		err = db.QueryRow(query, reserv.Username, reserv.Symbol, reserv.Order, reserv.Shares, reserv.Amount, reserv.Time).Scan(&rid)
	} else {
		err = db.QueryRow(query, reserv.Username, reserv.Symbol, reserv.Order, reserv.Shares, reserv.Amount, reserv.Time).Scan(&rid)
	}
	return
}

func UpdateUserStock(tx *sql.Tx, username string, symbol string, shares int, order models.OrderType) (err error) {
	stock, err := dbutils.QueryUserStock(username, symbol)
	if err != nil {
		if err == sql.ErrNoRows {
			query := "INSERT INTO stocks(username,symbol,shares) VALUES($1,$2,$3)"
			_, err = tx.Exec(query, username, symbol, shares)
			return
		}
		return
	}

	// adjust shares depending on order type
	if order == models.BUY {
		stock.Shares += shares
	} else {
		stock.Shares -= shares
	}

	query := "UPDATE stocks SET shares=$1 WHERE username=$2 AND symbol=$3"
	_, err = tx.Exec(query, stock.Shares, stock.Username, stock.Symbol)
	return
}

func UpdateUserMoney(tx *sql.Tx, username string, money int, order models.OrderType) (err error) {
	user, err := dbutils.QueryUser(username)
	if err != nil {
		return
	}

	if order == models.BUY {
		user.Money -= money
	} else {
		user.Money += money
	}

	query := "UPDATE users SET money=$1 WHERE username=$2"
	if tx == nil {
		_, err = db.Exec(query, user.Money, user.Username)
	} else {
		_, err = tx.Exec(query, user.Money, user.Username)
	}
	return
}

func RemoveReservation(tx *sql.Tx, rid int64) (err error) {
	query := "DELETE FROM reservations WHERE rid = $1"
	if tx == nil {
		_, err = db.Exec(query, rid)
	} else {
		_, err = tx.Exec(query, rid)
	}
	return
}

func RemoveOrder(rid int64, timeout time.Duration) {
	time.Sleep(timeout * time.Second)

	err := RemoveReservation(nil, rid)
	if err != nil {
		log.Println("Error removing reservation due to timeout.")
	}
}

func RemoveLastOrderTypeReservation(username string, orderType models.OrderType) (res models.Reservation, err error) {
	query := `DELETE FROM reservations WHERE rid IN ( 
				SELECT rid FROM reservations WHERE username=$1 AND type=$2 ORDER BY time DESC, rid DESC LIMIT(1)) 
				RETURNING rid, username, symbol, shares, amount, type, time`

	err = db.QueryRow(query, username, orderType).Scan(&res.ID, &res.Username, &res.Symbol, &res.Shares, &res.Amount, &res.Order, &res.Time)
	return
}

// func ExecuteSetBuyAmount(username string, symbol string, orderType string, buyAmount float64) (err error) {

// 	tx, err := db.Begin()

// 	err = SetUserOrderTypeAmount(tx, username, symbol, orderType, buyAmount, nil)
// 	if err != nil {
// 		utils.LogErr(err)
// 		return
// 	}

// 	err = UpdateUserMoney(tx, username, buyAmount, orderType, nil)
// 	if err != nil {
// 		utils.LogErr(err)
// 	}

// 	err = tx.Commit()
// 	if err != nil {
// 		utils.LogErr(err)
// 		tx.Rollback()
// 		return
// 	}

// 	return
// }

// func SetUserOrderTypeAmount(tx *sql.Tx, username string, stock string, orderType string, amount float64, channel chan error) (err error) {

// 	query := "INSERT INTO triggers(username, symbol, type, amount) VALUES($1,$2,$3,$4)"
// 	if tx != nil {
// 		_, err = tx.Exec(query, username, stock, orderType, amount)
// 	} else {
// 		_, err = db.Exec(query, username, stock, orderType, amount)
// 	}

// 	if err != nil {
// 		utils.LogErr(err)
// 	}

// 	if channel != nil {
// 		channel <- err
// 	}
// 	return
// }

// func RemoveUserStockTrigger(tx *sql.Tx, username string, stock string, orderType string, channel chan error) (err error) {

// 	query := "DELETE FROM triggers WHERE username=$1 AND symbol=$2 AND type=$3"
// 	if tx != nil {
// 		_, err = tx.Exec(query, username, stock, orderType)
// 	} else {
// 		_, err = db.Exec(query, username, stock, orderType)
// 	}

// 	if err != nil {
// 		utils.LogErr(err)
// 	}

// 	if channel != nil {
// 		channel <- err
// 	}
// 	return
// }

// func UpdateUserStockTriggerPrice(username string, stock string, orderType string, triggerPrice string) (err error) {

// 	query := "UPDATE triggers SET trigger_price=$1 WHERE username=$2 AND symbol=$3 AND type=$4"
// 	_, err = db.Exec(query, triggerPrice, username, stock, orderType)

// 	if err != nil {
// 		utils.LogErr(err)
// 	}

// 	return
// }

// func UpdateUserStockTriggerSharesAndPrice(tx *sql.Tx, username string, stock string, shares string, triggerPrice float64) (err error) {

// 	query := "UPDATE triggers SET shares=$1, trigger_price=$2 WHERE username=$3 AND symbol=$4"
// 	if tx == nil {
// 		_, err = db.Exec(query, shares, triggerPrice, username, stock)
// 	} else {
// 		_, err = tx.Exec(query, shares, triggerPrice, username, stock)
// 	}

// 	if err != nil {
// 		utils.LogErr(err)
// 	}

// 	return
// }

func CommitBuySellTransaction(res models.Reservation) (err error) {
	tx, err := db.Begin()
	if err != nil {
		return
	}

	err = UpdateUserStock(tx, res.Username, res.Symbol, res.Shares, res.Order)
	if err != nil {
		tx.Rollback()
		return
	}

	err = UpdateUserMoney(tx, res.Username, res.Amount, res.Order)
	if err != nil {
		tx.Rollback()
		return
	}

	err = RemoveReservation(tx, res.ID)
	if err != nil {
		tx.Rollback()
		return
	}

	err = tx.Commit()
	if err != nil {
		tx.Rollback()
		return
	}
	return
}

// func SetBuyTrigger(username string, symbol string, orderType string, triggerPrice string) (err error) {

// 	// decline if trigger exists with triggerPrice and buyAmount
// 	err = UpdateUserStockTriggerPrice(username, symbol, orderType, triggerPrice)
// 	if err != nil {
// 		utils.LogErr(err)
// 		return
// 	}

// 	return
// }

// func SetSellTrigger(username string, symbol string, totalValue float64, triggerPrice float64) (err error) {

// 	orderType := "sell"
// 	shares := int(totalValue / triggerPrice)
// 	sharesStr := strconv.Itoa(shares)

// 	tx, err := db.Begin()

// 	err1 := UpdateUserStock(tx, username, symbol, shares, orderType, nil)
// 	err2 := UpdateUserStockTriggerSharesAndPrice(tx, username, symbol, sharesStr, triggerPrice)

// 	if err != nil || err1 != nil || err2 != nil {
// 		tx.Rollback()
// 		err = errors.New("error querying within transaction")
// 		utils.LogErr(err)
// 		return
// 	}

// 	err = tx.Commit()
// 	if err != nil {
// 		utils.LogErr(err)
// 		tx.Rollback()
// 		return
// 	}

// 	return
// }

// func CancelSetTrigger(username string, symbol string, orderType string) (err error) {

// 	_, shares, totalValue, _, err := dbutils.QueryUserStockTrigger(username, symbol, orderType)
// 	if err != nil {
// 		// DB error or no trigger exists
// 		return
// 	}

// 	isSell := strings.Compare(orderType, "sell") == 0
// 	var err1 error = nil

// 	tx, err := db.Begin()

// 	if isSell && shares > 0 {
// 		orderType := "buy"
// 		//adds stock back
// 		err1 = UpdateUserStock(tx, username, symbol, int(shares), orderType, nil)
// 	} else {
// 		orderType := "sell"
// 		//adds money back
// 		err1 = UpdateUserMoney(tx, username, totalValue, orderType, nil)
// 	}

// 	err2 := RemoveUserStockTrigger(tx, username, symbol, orderType, nil)

// 	if err != nil || err1 != nil || err2 != nil {
// 		tx.Rollback()
// 		err = errors.New("error querying within transaction")
// 		return
// 	}

// 	err = tx.Commit()
// 	if err != nil {
// 		utils.LogErr(err)
// 		tx.Rollback()
// 		return
// 	}

// 	return
// }

// func ExecuteTrigger(username string, symbol string, shares string, totalValue float64, triggerValue float64, orderType string) (err error) {

// 	var sharesInt int
// 	isSellOrder := strings.Compare(orderType, "sell") == 0

// 	if !isSellOrder {
// 		sharesInt = int(totalValue / triggerValue)
// 	} else {
// 		sharesInt, _ = strconv.Atoi(shares)
// 	}

// 	tx, err := db.Begin()

// 	err1 := UpdateUserStock(tx, username, symbol, sharesInt, orderType, nil)
// 	if err1 != nil {
// 		utils.LogErr(err)
// 		tx.Rollback()
// 		return
// 	}

// 	err2 := RemoveUserStockTrigger(tx, username, symbol, orderType, nil)
// 	if err2 != nil {
// 		utils.LogErr(err2)
// 		tx.Rollback()
// 		return
// 	}

// 	if isSellOrder {
// 		err3 := UpdateUserMoney(tx, username, totalValue, orderType, nil)
// 		if err3 != nil {
// 			tx.Rollback()
// 			return
// 		}
// 	}

// 	err = tx.Commit()
// 	if err != nil {
// 		utils.LogErr(err)
// 		tx.Rollback()
// 		return
// 	}

// 	return
// }
