package transdb

import (
	"github.com/jackc/pgx"

	"common/logging"
	"common/models"
)

type TransactionDB struct {
	DB *pgx.ConnPool
	logger logging.Logger
}

func ScanTrigger(row *pgx.Row) (trig models.Trigger, err error) {
	err = row.Scan(&trig.ID, &trig.Username, &trig.Symbol, &trig.Order, &trig.Amount, &trig.TriggerPrice, &trig.Executable, &trig.Time)
	return
}

func ScanTriggerRows(rows *pgx.Rows) (trig models.Trigger, err error) {
	err = rows.Scan(&trig.ID, &trig.Username, &trig.Symbol, &trig.Order, &trig.Amount, &trig.TriggerPrice, &trig.Executable, &trig.Time)
	return
}

func (tdb *TransactionDB) QueryUserAvailableBalance(username string) (balance int, err error) {
	query := `SELECT (SELECT money FROM USERS WHERE username = $1) as available_balance;`
	err = tdb.DB.QueryRow(query, username).Scan(&balance)
	return
}

func (tdb *TransactionDB) QueryUserAvailableShares(username string, symbol string) (shares int, err error) {
	query := `SELECT (SELECT COALESCE(SUM(shares), 0) FROM Stocks WHERE username = $1 and symbol = $2)`
	err = tdb.DB.QueryRow(query, username, symbol).Scan(&shares)
	return
}

func (tdb *TransactionDB) QueryUser(username string) (user models.User, err error) {
	query := "SELECT uid, username, money FROM users WHERE username = $1"
	err = tdb.DB.QueryRow(query, username).Scan(&user.ID, &user.Username, &user.Money)
	return
}

func (tdb *TransactionDB) QueryUserStock(username string, symbol string) (stock models.Stock, err error) {

	query := "SELECT sid, username, symbol, shares FROM stocks WHERE username = $1 AND symbol = $2"
	err = tdb.DB.QueryRow(query, username, symbol).Scan(&stock.ID, &stock.Username, &stock.Symbol, &stock.Shares)
	return
}

func (tdb *TransactionDB) QueryStockTrigger(tid int64) (trig models.Trigger, err error) {
	query := "SELECT tid, username, symbol, type, amount, trigger_price, executable, time FROM triggers WHERE tid = $1"
	trig, err = ScanTrigger(tdb.DB.QueryRow(query, tid))
	return
}

func (tdb *TransactionDB) QueryUserTrigger(username string, symbol string, orderType models.OrderType) (trig models.Trigger, err error) {
	query := "SELECT tid, username, symbol, type, amount, trigger_price, executable, time FROM triggers WHERE username = $1 AND symbol=$2 AND type=$3"
	trig, err = ScanTrigger(tdb.DB.QueryRow(query, username, symbol, orderType))
	return
}

func (tdb *TransactionDB) QueryReservation(rid int64) (res models.Reservation, err error) {
	query := "SELECT rid, username, symbol, shares, amount, type, time FROM reservations WHERE rid=$1"
	err = tdb.DB.QueryRow(query, rid).Scan(&res.ID, &res.Username, &res.Symbol, &res.Shares, &res.Amount, &res.Order, &res.Time)
	return
}

func (tdb *TransactionDB) QueryLastReservation(username string, resType models.OrderType) (res models.Reservation, err error) {
	query := "SELECT rid, username, symbol, shares, amount, type, time FROM reservations WHERE username=$1 and type=$2 ORDER BY (time) DESC, rid DESC LIMIT 1"
	err = tdb.DB.QueryRow(query, username, resType).Scan(&res.ID, &res.Username, &res.Symbol, &res.Shares, &res.Amount, &res.Order, &res.Time)
	return
}
