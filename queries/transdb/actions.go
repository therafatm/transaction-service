package transdb

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"common/logging"
	"common/models"
	"common/utils"
	"transaction_service/queries/utils"

	"github.com/go-redis/redis"
)

func NewQuoteCacheConnection() (cache *redis.Client) {
	host := os.Getenv("REDIS_HOST")
	port := os.Getenv("REDIS_PORT")
	addr := fmt.Sprintf("%s:%s", host, port)

	cache = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	_, err := cache.Ping().Result()
	if err != nil {
		utils.LogErr(err, "Error connecting to quote cache.")
		panic(err)
	}

	return
}

func NewTransactionDBConnection(host string, port string) (tdb *TransactionDB) {
	user := os.Getenv("PGUSER")
	password := os.Getenv("PGPASSWORD")
	dbname := os.Getenv("TRANS_DB")
	config := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", host, port, user, password, dbname)

	db, err := sql.Open("postgres", config)
	if err != nil {
		utils.LogErr(err, "Error connecting to DB.")
		panic(err)
	}

	logger := logging.NewLoggerConnection()

	tdb = &TransactionDB{DB: db, logger: logger}

	return
}

func (tdb *TransactionDB) ClearUsers() (err error) {
	query := "DELETE FROM Users"
	_, err = tdb.DB.Exec(query)
	return
}

func (tdb *TransactionDB) InsertUser(user models.User) (res sql.Result, err error) {
	//add new user
	query := "INSERT INTO users(username, money) VALUES($1,$2)"
	res, err = tdb.DB.Exec(query, user.Username, user.Money)
	return
}

func (tdb *TransactionDB) UpdateUser(user models.User) (res sql.Result, err error) {
	query := "UPDATE users SET money = $1 WHERE username = $2"
	money := fmt.Sprintf("%d", user.Money)
	res, err = tdb.DB.Exec(query, money, user.Username)
	return
}

func (tdb *TransactionDB) AddReservation(tx *sql.Tx, res models.Reservation) (rid int64, err error) {
	query := "INSERT INTO reservations(username, symbol, type, shares, amount, time) VALUES($1,$2,$3,$4,$5,$6) RETURNING rid"
	if tx == nil {
		err = tdb.DB.QueryRow(query, res.Username, res.Symbol, res.Order, res.Shares, res.Amount, res.Time).Scan(&rid)
	} else {
		err = tx.QueryRow(query, res.Username, res.Symbol, res.Order, res.Shares, res.Amount, res.Time).Scan(&rid)
	}
	return
}

func (tdb *TransactionDB) UpdateUserStock(tx *sql.Tx, username string, symbol string, shares int, order models.OrderType) (err error) {
	stock, err := tdb.QueryUserStock(username, symbol)
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

func (tdb *TransactionDB) UpdateUserMoney(tx *sql.Tx, username string, money int, order models.OrderType, trans string) (err error) {
	user, err := tdb.QueryUser(username)
	if err != nil {
		return
	}

	if order == models.BUY {
		user.Money -= money
		tdb.logger.LogTransaction("remove", username, money, trans)

	} else {
		user.Money += money
		tdb.logger.LogTransaction("add", username, money, trans)
	}

	query := "UPDATE users SET money=$1 WHERE username=$2"
	if tx == nil {
		_, err = tdb.DB.Exec(query, user.Money, user.Username)
	} else {
		_, err = tx.Exec(query, user.Money, user.Username)
	}
	return
}

func (tdb *TransactionDB) RemoveReservation(tx *sql.Tx, rid int64) (err error) {
	query := "DELETE FROM reservations WHERE rid = $1"
	if tx == nil {
		_, err = tdb.DB.Exec(query, rid)
	} else {
		_, err = tx.Exec(query, rid)
	}
	return
}

func (tdb *TransactionDB) RemoveOrder(rid int64, timeout time.Duration) {
	time.Sleep(timeout * time.Second)

	err := tdb.RemoveReservation(nil, rid)
	if err != nil {
		log.Println("Error removing reservation due to timeout.")
	}
}

func (tdb *TransactionDB) RemoveLastOrderTypeReservation(username string, orderType models.OrderType) (res models.Reservation, err error) {
	query := `DELETE FROM reservations WHERE rid IN ( 
				SELECT rid FROM reservations WHERE username=$1 AND type=$2 ORDER BY time DESC, rid DESC LIMIT(1)) 
				RETURNING rid, username, symbol, shares, amount, type, time`

	err = tdb.DB.QueryRow(query, username, orderType).Scan(&res.ID, &res.Username, &res.Symbol, &res.Shares, &res.Amount, &res.Order, &res.Time)
	return
}

func (tdb *TransactionDB) SetUserOrderTypeAmount(tx *sql.Tx, username string, symbol string, orderType models.OrderType, amount int) (tid int64, err error) {
	query := "INSERT INTO triggers(username, symbol, type, amount, trigger_price, executable, time) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING tid"
	t := time.Now().Unix()
	if tx != nil {
		err = tx.QueryRow(query, username, symbol, orderType, amount, 0, false, t).Scan(&tid)
	} else {
		err = tdb.DB.QueryRow(query, username, symbol, orderType, amount, 0, false, t).Scan(&tid)
	}
	return
}

func (tdb *TransactionDB) RemoveUserStockTrigger(tx *sql.Tx, tid int64) (trig models.Trigger, err error) {
	query := `DELETE FROM triggers WHERE tid=$1 RETURNING tid, username, symbol, type, amount, trigger_price, executable, time`
	if tx != nil {
		trig, err = ScanTrigger(tx.QueryRow(query, tid))
	} else {
		trig, err = ScanTrigger(tdb.DB.QueryRow(query, tid))
	}
	return
}

func (tdb *TransactionDB) UpdateTrigger(trig models.Trigger) (err error) {
	query := "UPDATE Triggers SET username=$2, symbol=$3, type=$4, amount=$5, trigger_price=$6, executable=$7, time=$8 WHERE tid=$1"
	_, err = tdb.DB.Exec(query, trig.ID, trig.Username, trig.Symbol, trig.Order, trig.Amount, trig.TriggerPrice, trig.Executable, trig.Time)
	return
}

func (tdb *TransactionDB) UpdateUserStockTriggerPrice(username string, stock string, orderType string, triggerPrice string) (err error) {
	query := "UPDATE triggers SET trigger_price=$1 WHERE username=$2 AND symbol=$3 AND type=$4"
	_, err = tdb.DB.Exec(query, triggerPrice, username, stock, orderType)
	return
}

func (tdb *TransactionDB) CommitSetOrderTransaction(username string, symbol string, orderType models.OrderType, amount int, trans string) (tid int64, err error) {
	tx, err := tdb.DB.Begin()
	if err != nil {
		return
	}

	if orderType == models.BUY {
		err = tdb.UpdateUserMoney(tx, username, amount, orderType, trans)
	} else {
		//TODO: check for sell
		err = tdb.UpdateUserStock(tx, username, symbol, amount, orderType)
	}
	if err != nil {
		tx.Rollback()
		return
	}

	//TODO: check for sell
	tid, err = tdb.SetUserOrderTypeAmount(tx, username, symbol, orderType, amount)
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

func (tdb *TransactionDB) CancelOrderTransaction(trig models.Trigger, trans string) (rtrig models.Trigger, err error) {
	tx, err := tdb.DB.Begin()
	if err != nil {
		return
	}

	if trig.Order == models.BUY {
		err = tdb.UpdateUserMoney(tx, trig.Username, trig.Amount, models.SELL, trans)
	} else {
		err = tdb.UpdateUserStock(tx, trig.Username, trig.Symbol, trig.Amount, models.BUY)
	}
	if err != nil {
		tx.Rollback()
		return
	}

	rtrig, err = tdb.RemoveUserStockTrigger(tx, trig.ID)
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

func (tdb *TransactionDB) CommitBuySellTransaction(res models.Reservation, trans string) (err error) {
	tx, err := tdb.DB.Begin()
	if err != nil {
		return
	}

	err = tdb.UpdateUserStock(tx, res.Username, res.Symbol, res.Shares, res.Order)
	if err != nil {
		tx.Rollback()
		return
	}

	err = tdb.UpdateUserMoney(tx, res.Username, res.Amount, res.Order, trans)
	if err != nil {
		tx.Rollback()
		return
	}

	err = tdb.RemoveReservation(tx, res.ID)
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

func (tdb *TransactionDB) QueryAndExecuteCurrentTriggers(quoteCache *redis.Client, trans string) (rTrigs []models.Trigger, err error) {
	query := `SELECT tid, username, symbol, type, amount, trigger_price, executable, time FROM triggers WHERE executable=TRUE`

	rows, err := tdb.DB.Query(query)
	if err != nil {
		return
	}

	defer rows.Close()

	for rows.Next() {
		trig, err := ScanTriggerRows(rows)
		if err == nil {
			quote, err := dbutils.QueryQuotePrice(quoteCache, tdb.logger, trig.Username, trig.Symbol, trans)
			if err == nil {
				if trig.Order == models.BUY {
					if quote <= trig.TriggerPrice {
						trig, err = tdb.ExecuteTrigger(trig, quote, trans)
					}

				} else {
					if quote >= trig.TriggerPrice {
						trig, err = tdb.ExecuteTrigger(trig, quote, trans)
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

func (tdb *TransactionDB) ExecuteTrigger(trig models.Trigger, quote int, trans string) (rtrig models.Trigger, err error) {
	tx, err := tdb.DB.Begin()
	if err != nil {
		return
	}

	if trig.Order == models.BUY {
		shares := trig.Amount / quote
		remainder := trig.Amount - (shares * quote)

		// add stock
		err = tdb.UpdateUserStock(tx, trig.Username, trig.Symbol, shares, trig.Order)
		if err != nil {
			tx.Rollback()
			return
		}

		//add remainder back
		err = tdb.UpdateUserMoney(tx, trig.Username, remainder, models.SELL, trans)
		if err != nil {
			tx.Rollback()
			return
		}

	} else {
		// add spendings
		err = tdb.UpdateUserMoney(tx, trig.Username, trig.Amount, trig.Order, trans)
		if err != nil {
			tx.Rollback()
			return
		}
	}
	rtrig, err = tdb.RemoveUserStockTrigger(tx, trig.ID)
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
