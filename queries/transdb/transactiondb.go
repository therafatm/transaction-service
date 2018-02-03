package transdb

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	//"transaction_service/logger"
	"transaction_service/queries/models"
	"transaction_service/queries/utils"
)

type TransactionDataStore interface {
	QueryUserAvailableBalance(username string) (int, error)
	QueryUserAvailableShares(username string, symbol string) (shares int, err error)
	QueryUser(username string) (user models.User, err error)
	QueryUserStock(username string, symbol string) (stock models.Stock, err error)
	QueryStockTrigger(tid int64) (trig models.Trigger, err error)
	QueryUserTrigger(username string, symbol string, orderType models.OrderType) (trig models.Trigger, err error)
	QueryReservation(rid int64) (res models.Reservation, err error)
	QueryLastReservation(username string, resType models.OrderType) (res models.Reservation, err error)
	ClearUsers() (err error)
	InsertUser(user models.User) (res sql.Result, err error)
	UpdateUser(user models.User) (res sql.Result, err error)
	AddReservation(tx *sql.Tx, res models.Reservation) (rid int64, err error)
	UpdateUserStock(tx *sql.Tx, username string, symbol string, shares int, order models.OrderType) (err error)
	UpdateUserMoney(tx *sql.Tx, username string, money int, order models.OrderType, trans string) (err error)
	RemoveReservation(tx *sql.Tx, rid int64) (err error)
	RemoveOrder(rid int64, timeout time.Duration)
	RemoveLastOrderTypeReservation(username string, orderType models.OrderType) (res models.Reservation, err error)
	SetUserOrderTypeAmount(tx *sql.Tx, username string, symbol string, orderType models.OrderType, amount int) (tid int64, err error)
	RemoveUserStockTrigger(tx *sql.Tx, tid int64) (trig models.Trigger, err error)
	UpdateTrigger(trig models.Trigger) (err error)
	UpdateUserStockTriggerPrice(username string, stock string, orderType string, triggerPrice string) (err error)
	CommitSetOrderTransaction(username string, symbol string, orderType models.OrderType, amount int, trans string) (tid int64, err error)
	CancelOrderTransaction(trig models.Trigger, trans string) (rtrig models.Trigger, err error)
	CommitBuySellTransaction(res models.Reservation, trans string) (err error)
	QueryAndExecuteCurrentTriggers(trans string) (rTrigs []models.Trigger, err error)
	ExecuteTrigger(trig models.Trigger, quote int, trans string) (rtrig models.Trigger, err error)
}

type TransactionDB struct {
	*sql.DB
}

func NewTransactionDBConnection(config string) (tdb TransactionDB, err error) {
	db, err := sql.Open("postgres", config)
	if err != nil {
		return
	}
	tdb = TransactionDB{db}
	return
}

func ScanTrigger(row *sql.Row) (trig models.Trigger, err error) {
	err = row.Scan(&trig.ID, &trig.Username, &trig.Symbol, &trig.Order, &trig.Amount, &trig.TriggerPrice, &trig.Executable, &trig.Time)
	return
}

func ScanTriggerRows(rows *sql.Rows) (trig models.Trigger, err error) {
	err = rows.Scan(&trig.ID, &trig.Username, &trig.Symbol, &trig.Order, &trig.Amount, &trig.TriggerPrice, &trig.Executable, &trig.Time)
	return
}

func (db *TransactionDB) QueryUserAvailableBalance(username string) (balance int, err error) {
	query := `SELECT (SELECT money FROM USERS WHERE username = $1) as available_balance;`
	err = db.QueryRow(query, username).Scan(&balance)
	return
}

func (db *TransactionDB) QueryUserAvailableShares(username string, symbol string) (shares int, err error) {
	query := `SELECT (SELECT COALESCE(SUM(shares), 0) FROM Stocks WHERE username = $1 and symbol = $2)`
	err = db.QueryRow(query, username, symbol).Scan(&shares)
	return
}

func (db *TransactionDB) QueryUser(username string) (user models.User, err error) {
	query := "SELECT uid, username, money FROM users WHERE username = $1"
	err = db.QueryRow(query, username).Scan(&user.ID, &user.Username, &user.Money)
	return
}

func (db *TransactionDB) QueryUserStock(username string, symbol string) (stock models.Stock, err error) {
	query := "SELECT sid, username, symbol, shares FROM stocks WHERE username = $1 AND symbol = $2"
	err = db.QueryRow(query, username, symbol).Scan(&stock.ID, &stock.Username, &stock.Symbol, &stock.Shares)
	return
}

func (db *TransactionDB) QueryStockTrigger(tid int64) (trig models.Trigger, err error) {
	query := "SELECT tid, username, symbol, type, amount, trigger_price, executable, time FROM triggers WHERE tid = $1"
	trig, err = ScanTrigger(db.QueryRow(query, tid))
	return
}

func (db *TransactionDB) QueryUserTrigger(username string, symbol string, orderType models.OrderType) (trig models.Trigger, err error) {
	query := "SELECT tid, username, symbol, type, amount, trigger_price, executable, time FROM triggers WHERE username = $1 AND symbol=$2 AND type=$3"
	trig, err = ScanTrigger(db.QueryRow(query, username, symbol, orderType))
	return
}

func (db *TransactionDB) QueryReservation(rid int64) (res models.Reservation, err error) {
	query := "SELECT rid, username, symbol, shares, amount, type, time FROM reservations WHERE rid=$1"
	err = db.QueryRow(query, rid).Scan(&res.ID, &res.Username, &res.Symbol, &res.Shares, &res.Amount, &res.Order, &res.Time)
	return
}

func (db *TransactionDB) QueryLastReservation(username string, resType models.OrderType) (res models.Reservation, err error) {
	query := "SELECT rid, username, symbol, shares, amount, type, time FROM reservations WHERE username=$1 and type=$2 ORDER BY (time) DESC, rid DESC LIMIT 1"
	err = db.QueryRow(query, username, resType).Scan(&res.ID, &res.Username, &res.Symbol, &res.Shares, &res.Amount, &res.Order, &res.Time)
	return
}

func (db *TransactionDB) ClearUsers() (err error) {
	query := "DELETE FROM Users"
	_, err = db.Exec(query)
	return
}

func (db *TransactionDB) InsertUser(user models.User) (res sql.Result, err error) {
	//add new user
	query := "INSERT INTO users(username, money) VALUES($1,$2)"
	res, err = db.Exec(query, user.Username, user.Money)
	return
}

func (db *TransactionDB) UpdateUser(user models.User) (res sql.Result, err error) {
	query := "UPDATE users SET money = $1 WHERE username = $2"
	money := fmt.Sprintf("%d", user.Money)
	res, err = db.Exec(query, money, user.Username)
	return
}

func (db *TransactionDB) AddReservation(tx *sql.Tx, res models.Reservation) (rid int64, err error) {
	query := "INSERT INTO reservations(username, symbol, type, shares, amount, time) VALUES($1,$2,$3,$4,$5,$6) RETURNING rid"
	if tx == nil {
		err = db.QueryRow(query, res.Username, res.Symbol, res.Order, res.Shares, res.Amount, res.Time).Scan(&rid)
	} else {
		err = tx.QueryRow(query, res.Username, res.Symbol, res.Order, res.Shares, res.Amount, res.Time).Scan(&rid)
	}
	return
}

func (db *TransactionDB) UpdateUserStock(tx *sql.Tx, username string, symbol string, shares int, order models.OrderType) (err error) {
	stock, err := db.QueryUserStock(username, symbol)
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

func (db *TransactionDB) UpdateUserMoney(tx *sql.Tx, username string, money int, order models.OrderType, trans string) (err error) {
	user, err := db.QueryUser(username)
	if err != nil {
		return
	}

	if order == models.BUY {
		user.Money -= money
		//logger.LogTransaction("remove", username, money, trans)

	} else {
		user.Money += money
		//logger.LogTransaction("add", username, money, trans)
	}

	query := "UPDATE users SET money=$1 WHERE username=$2"
	if tx == nil {
		_, err = db.Exec(query, user.Money, user.Username)
	} else {
		_, err = tx.Exec(query, user.Money, user.Username)
	}
	return
}

func (db *TransactionDB) RemoveReservation(tx *sql.Tx, rid int64) (err error) {
	query := "DELETE FROM reservations WHERE rid = $1"
	if tx == nil {
		_, err = db.Exec(query, rid)
	} else {
		_, err = tx.Exec(query, rid)
	}
	return
}

func (db *TransactionDB) RemoveOrder(rid int64, timeout time.Duration) {
	time.Sleep(timeout * time.Second)

	err := db.RemoveReservation(nil, rid)
	if err != nil {
		log.Println("Error removing reservation due to timeout.")
	}
}

func (db *TransactionDB) RemoveLastOrderTypeReservation(username string, orderType models.OrderType) (res models.Reservation, err error) {
	query := `DELETE FROM reservations WHERE rid IN ( 
				SELECT rid FROM reservations WHERE username=$1 AND type=$2 ORDER BY time DESC, rid DESC LIMIT(1)) 
				RETURNING rid, username, symbol, shares, amount, type, time`

	err = db.QueryRow(query, username, orderType).Scan(&res.ID, &res.Username, &res.Symbol, &res.Shares, &res.Amount, &res.Order, &res.Time)
	return
}

func (db *TransactionDB) SetUserOrderTypeAmount(tx *sql.Tx, username string, symbol string, orderType models.OrderType, amount int) (tid int64, err error) {
	query := "INSERT INTO triggers(username, symbol, type, amount, trigger_price, executable, time) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING tid"
	t := time.Now().Unix()
	if tx != nil {
		err = tx.QueryRow(query, username, symbol, orderType, amount, 0, false, t).Scan(&tid)
	} else {
		err = db.QueryRow(query, username, symbol, orderType, amount, 0, false, t).Scan(&tid)
	}
	return
}

func (db *TransactionDB) RemoveUserStockTrigger(tx *sql.Tx, tid int64) (trig models.Trigger, err error) {
	query := `DELETE FROM triggers WHERE tid=$1 RETURNING tid, username, symbol, type, amount, trigger_price, executable, time`
	if tx != nil {
		trig, err = ScanTrigger(tx.QueryRow(query, tid))
	} else {
		trig, err = ScanTrigger(db.QueryRow(query, tid))
	}
	return
}

func (db *TransactionDB) UpdateTrigger(trig models.Trigger) (err error) {
	query := "UPDATE Triggers SET username=$2, symbol=$3, type=$4, amount=$5, trigger_price=$6, executable=$7, time=$8 WHERE tid=$1"
	_, err = db.Exec(query, trig.ID, trig.Username, trig.Symbol, trig.Order, trig.Amount, trig.TriggerPrice, trig.Executable, trig.Time)
	return
}

func (db *TransactionDB) UpdateUserStockTriggerPrice(username string, stock string, orderType string, triggerPrice string) (err error) {
	query := "UPDATE triggers SET trigger_price=$1 WHERE username=$2 AND symbol=$3 AND type=$4"
	_, err = db.Exec(query, triggerPrice, username, stock, orderType)
	return
}

func (db *TransactionDB) CommitSetOrderTransaction(username string, symbol string, orderType models.OrderType, amount int, trans string) (tid int64, err error) {
	tx, err := db.Begin()
	if err != nil {
		tx.Rollback()
		return
	}

	if orderType == models.BUY {
		err = db.UpdateUserMoney(tx, username, amount, orderType, trans)
	} else {
		//TODO: check for sell
		err = db.UpdateUserStock(tx, username, symbol, amount, orderType)
	}
	if err != nil {
		tx.Rollback()
		return
	}

	//TODO: check for sell
	tid, err = db.SetUserOrderTypeAmount(tx, username, symbol, orderType, amount)
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

func (db *TransactionDB) CancelOrderTransaction(trig models.Trigger, trans string) (rtrig models.Trigger, err error) {
	tx, err := db.Begin()
	if err != nil {
		tx.Rollback()
		return
	}

	if trig.Order == models.BUY {
		err = db.UpdateUserMoney(tx, trig.Username, trig.Amount, models.SELL, trans)
	} else {
		err = db.UpdateUserStock(tx, trig.Username, trig.Symbol, trig.Amount, models.BUY)
	}
	if err != nil {
		tx.Rollback()
		return
	}

	rtrig, err = db.RemoveUserStockTrigger(tx, trig.ID)
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

func (db *TransactionDB) CommitBuySellTransaction(res models.Reservation, trans string) (err error) {
	tx, err := db.Begin()
	if err != nil {
		return
	}

	err = db.UpdateUserStock(tx, res.Username, res.Symbol, res.Shares, res.Order)
	if err != nil {
		tx.Rollback()
		return
	}

	err = db.UpdateUserMoney(tx, res.Username, res.Amount, res.Order, trans)
	if err != nil {
		tx.Rollback()
		return
	}

	err = db.RemoveReservation(tx, res.ID)
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

func (db *TransactionDB) QueryAndExecuteCurrentTriggers(trans string) (rTrigs []models.Trigger, err error) {
	query := `SELECT tid, username, symbol, type, amount, trigger_price, executable, time FROM triggers WHERE executable=TRUE`

	rows, err := db.Query(query)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		trig, err := ScanTriggerRows(rows)
		if err == nil {
			quote, err := dbutils.QueryQuotePrice(trig.Username, trig.Symbol, trans)
			if err == nil {
				if trig.Order == models.BUY {
					if quote <= trig.TriggerPrice {
						trig, err = db.ExecuteTrigger(trig, quote, trans)
					}

				} else {
					if quote >= trig.TriggerPrice {
						trig, err = db.ExecuteTrigger(trig, quote, trans)
					}
				}
				if err == nil {
					rTrigs = append(rTrigs, trig)
				}
			}
		}
	}
	return
}

func (db *TransactionDB) ExecuteTrigger(trig models.Trigger, quote int, trans string) (rtrig models.Trigger, err error) {
	tx, err := db.Begin()
	if err != nil {
		return
	}

	if trig.Order == models.BUY {
		shares := trig.Amount / quote
		remainder := trig.Amount - (shares * quote)

		// add stock
		err = db.UpdateUserStock(tx, trig.Username, trig.Symbol, shares, trig.Order)
		if err != nil {
			tx.Rollback()
			return
		}

		//add remainder back
		err = db.UpdateUserMoney(tx, trig.Username, remainder, models.SELL, trans)
		if err != nil {
			tx.Rollback()
			return
		}

	} else {
		// add spendings
		err = db.UpdateUserMoney(tx, trig.Username, trig.Amount, trig.Order, trans)
		if err != nil {
			tx.Rollback()
			return
		}
	}
	rtrig, err = db.RemoveUserStockTrigger(tx, trig.ID)
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
