package transdb

import (
	"common/models"
	"time"

	"github.com/go-redis/redis"
	"github.com/jackc/pgx"
)

//TODO: think about splitting queries and actions again
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
	InsertUser(user models.User) (res pgx.CommandTag, err error)
	UpdateUser(user models.User) (res pgx.CommandTag, err error)
	AddReservation(tx *pgx.Tx, res models.Reservation) (rid int64, err error)
	UpdateUserStock(tx *pgx.Tx, username string, symbol string, shares int, order models.OrderType) (err error)
	UpdateUserMoney(tx *pgx.Tx, username string, money int, order models.OrderType, trans string) (err error)
	RemoveReservation(tx *pgx.Tx, rid int64) (err error)
	RemoveOrder(rid int64, timeout time.Duration)
	RemoveLastOrderTypeReservation(username string, orderType models.OrderType) (res models.Reservation, err error)
	SetUserOrderTypeAmount(tx *pgx.Tx, username string, symbol string, orderType models.OrderType, amount int) (tid int64, err error)
	RemoveUserStockTrigger(tx *pgx.Tx, tid int64) (trig models.Trigger, err error)
	UpdateTrigger(trig models.Trigger) (err error)
	UpdateUserStockTriggerPrice(username string, stock string, orderType string, triggerPrice string) (err error)
	CommitSetOrderTransaction(username string, symbol string, orderType models.OrderType, amount int, trans string) (tid int64, err error)
	CancelOrderTransaction(trig models.Trigger, trans string) (rtrig models.Trigger, err error)
	CommitBuySellTransaction(res models.Reservation, trans string) (err error)
	QueryAndExecuteCurrentTriggers(quoteCache *redis.Client, trans string) (rTrigs []models.Trigger, err error)
	QueryAllUserTriggers(username string) (trigs []models.Trigger, err error)
	ExecuteTrigger(trig models.Trigger, quote int, trans string) (rtrig models.Trigger, err error)
}
