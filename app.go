package main

import (
	"common/logging"
	"common/models"
	"common/utils"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
	"transaction_service/queries/transdb"
	"transaction_service/queries/utils"

	"github.com/go-redis/redis"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type Env struct {
	logger     logging.Logger
	tdb        transdb.TransactionDataStore
	quoteCache *redis.Client
	databases  (map[int]transdb.TransactionDataStore)
}

type extendedHandlerFunc func(http.ResponseWriter, *http.Request, logging.Command)

func hash(s string) int {
	h := fnv.New32a()
	h.Write([]byte(s))
	return int(h.Sum32())
}

func (env *Env) respondWithError(w http.ResponseWriter, code int, err error, message string, command logging.Command, vars map[string]string) {
	env.logger.LogErrorEvent(command, vars, message)
	utils.LogErrSkip(err, message, 2) // skip 2 stack frames to get actual caller
	env.respondWithJSON(w, code, map[string]string{"error": err.Error(), "message": message})
}

func (env *Env) respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

//TODO: refactor  + test
func (env *Env) getQuoute(w http.ResponseWriter, r *http.Request, command logging.Command) {
	vars := mux.Vars(r)
	price, err := dbutils.QueryQuotePrice(env.quoteCache, env.logger, vars["username"], vars["symbol"], vars["trans"])
	if err != nil {
		errMsg := fmt.Sprintf("Error getting quote for %s and %s", vars["username"], vars["symbol"])
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}
	env.respondWithJSON(w, http.StatusOK, map[string]string{"price": strconv.Itoa(price), "symbol": vars["symbol"]})
}

//TODO: refactor
func (env *Env) clearUsers(w http.ResponseWriter, r *http.Request, command logging.Command) {
	vars := mux.Vars(r)
	err := env.tdb.ClearUsers()
	if err != nil {
		errMsg := "Failed to clear users"
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Cleared users succesfully."))
}

func (env *Env) addUser(w http.ResponseWriter, r *http.Request, command logging.Command) {
	vars := mux.Vars(r)
	username := vars["username"]
	moneyStr := vars["money"]
	errMsg := fmt.Sprintf("Failed to add user %s", username)

	money, err := strconv.Atoi(moneyStr)
	if err != nil {
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	tdb := env.databases[hash(username)%len(env.databases)]

	user, err := tdb.QueryUser(username)

	if err != nil && err == sql.ErrNoRows {
		//user no exist
		newUser := models.User{Username: username, Money: money}
		_, err := tdb.InsertUser(newUser)
		if err != nil {
			env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
			return
		}

	} else if err != nil {
		// error
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return

	} else {
		// user exists
		user.Money += money
		_, err = tdb.UpdateUser(user)

		if err != nil {
			env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
			return
		}
	}

	user, err = tdb.QueryUser(username)
	if err != nil {
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	env.respondWithJSON(w, http.StatusOK, user)
}

func (env *Env) availableBalance(w http.ResponseWriter, r *http.Request, command logging.Command) {
	vars := mux.Vars(r)
	username := vars["username"]

	tdb := env.databases[hash(username)%len(env.databases)]

	_, err := tdb.QueryUser(username)
	if err != nil && err == sql.ErrNoRows {
		errMsg := fmt.Sprintf("No such user %s exists.", username)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	} else if err != nil {
		errMsg := fmt.Sprintf("Error retrieving user %s.", username)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	balance, err := tdb.QueryUserAvailableBalance(username)
	if err != nil {
		errMsg := fmt.Sprintf("Error getting user available balance for %s.", username)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}
	var m map[string]int
	m = make(map[string]int)
	m["balance"] = balance

	env.respondWithJSON(w, http.StatusOK, m)
}

func (env *Env) availableShares(w http.ResponseWriter, r *http.Request, command logging.Command) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]

	tdb := env.databases[hash(username)%len(env.databases)]

	_, err := tdb.QueryUser(username)
	if err != nil && err == sql.ErrNoRows {
		errMsg := fmt.Sprintf("No such user %s exists.", username)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	} else if err != nil {
		errMsg := fmt.Sprintf("Error retrieving user %s.", username)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	balance, err := tdb.QueryUserAvailableShares(username, symbol)
	if err != nil {
		errMsg := fmt.Sprintf("Error getting user available shares for %s: %s.", username, symbol)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}
	var m map[string]int
	m = make(map[string]int)
	m["shares"] = balance

	env.respondWithJSON(w, http.StatusOK, m)
}

func (env *Env) buyOrder(w http.ResponseWriter, r *http.Request, command logging.Command) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]
	trans := vars["trans"]
	tdb := env.databases[hash(username)%len(env.databases)]

	buyAmount, err := strconv.Atoi(vars["amount"])
	if err != nil {
		errMsg := fmt.Sprintf("Invalid amount %s.", vars["amount"])
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	balance, err := tdb.QueryUserAvailableBalance(username)

	// check that user exists and has enough money
	if err != nil {
		if err == sql.ErrNoRows {
			errMsg := fmt.Sprintf("Failed to find user %s.", username)
			env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
			return
		}

		errMsg := fmt.Sprintf("Error getting user data for %s.", username)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	if balance < buyAmount {
		errMsg := fmt.Sprintf("User does not have enough money to complete order %d < %d.", balance, buyAmount)
		err = errors.New("Error not enough money.")
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	quote, err := dbutils.QueryQuotePrice(env.quoteCache, env.logger, username, symbol, trans)
	if err != nil {
		errMsg := fmt.Sprintf("Error getting quote from quote server for %s: %s.", username, symbol)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	reservation := models.Reservation{Username: username, Symbol: symbol, Order: models.BUY}
	reservation.Shares = buyAmount / quote
	reservation.Amount = reservation.Shares * quote
	reservation.Time = time.Now().Unix()

	rid, err := tdb.AddReservation(nil, reservation)
	if err != nil {
		errMsg := "Error setting buy order."
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	reserv, err := tdb.QueryReservation(rid)
	if err != nil {
		errMsg := "Error reservation not found after insert."
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	env.respondWithJSON(w, http.StatusOK, reserv)

	// remove reservation if not bought within 60 seconds
	go tdb.RemoveOrder(rid, 60)
}

func (env *Env) sellOrder(w http.ResponseWriter, r *http.Request, command logging.Command) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]
	trans := vars["trans"]
	tdb := env.databases[hash(username)%len(env.databases)]

	sellAmount, err := strconv.Atoi(vars["amount"])
	if err != nil {
		errMsg := fmt.Sprintf("Invalid amount %s.", vars["amount"])
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	quote, err := dbutils.QueryQuotePrice(env.quoteCache, env.logger, username, symbol, trans)
	if err != nil {
		errMsg := fmt.Sprintf("Error getting quote from quote server for %s: %s.", username, symbol)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	sharesToSell := sellAmount / quote

	availableShares, err := tdb.QueryUserAvailableShares(username, symbol)
	if err != nil {
		errMsg := fmt.Sprintf("Error querying available shares for %s: %s.", username, symbol)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	if availableShares < sharesToSell {
		errMsg := fmt.Sprintf("User does not have enough shares to complete order %d < %d", availableShares, sharesToSell)
		err = errors.New("Error not enough shares.")
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	reservation := models.Reservation{Username: username, Symbol: symbol, Order: models.SELL}
	reservation.Shares = sharesToSell
	reservation.Amount = reservation.Shares * quote
	reservation.Time = time.Now().Unix()

	rid, err := tdb.AddReservation(nil, reservation)
	if err != nil {
		errMsg := "Error setting sell order."
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	reserv, err := tdb.QueryReservation(rid)
	if err != nil {
		errMsg := "Error reservation not found after insert."
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	env.respondWithJSON(w, http.StatusOK, reserv)

	// remove reservation if not bought within 60 seconds
	go tdb.RemoveOrder(rid, 60)
}

func (env *Env) commitOrder(w http.ResponseWriter, r *http.Request, orderType models.OrderType, command logging.Command) {
	var vars = mux.Vars(r)
	username := vars["username"]
	trans := vars["trans"]
	tdb := env.databases[hash(username)%len(env.databases)]

	res, err := tdb.QueryLastReservation(username, orderType)
	if err != nil && err == sql.ErrNoRows {
		errMsg := fmt.Sprintf("No reserved %s order to commit.", orderType)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	} else if err != nil {
		errMsg := fmt.Sprintf("Error finding last %s reservation.", orderType)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	var balance int
	var amount int

	if orderType == models.BUY {
		balance, err = tdb.QueryUserAvailableBalance(username)
		amount = res.Amount

	} else {
		balance, err = tdb.QueryUserAvailableShares(username, res.Symbol)
		amount = res.Shares
	}

	// check that user exists and has enough resources
	if err != nil {
		if err == sql.ErrNoRows {
			errMsg := fmt.Sprintf("Failed to find user %s.", username)
			env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
			return
		}

		errMsg := fmt.Sprintf("Error getting user data for %s.", username)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	if balance < amount {
		errMsg := fmt.Sprintf("User does not have enough resources to complete order %d < %d.", balance, amount)
		err = errors.New("Error not enough resources.")
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	err = tdb.CommitBuySellTransaction(res, trans)
	if err != nil {
		errMsg := fmt.Sprintf("Error commiting  %s order.", orderType)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	stock, err := tdb.QueryUserStock(res.Username, res.Symbol)
	if err != nil {
		errMsg := "Error could not find updated stock."
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
	}

	env.respondWithJSON(w, http.StatusOK, stock)
	return
}

func (env *Env) commitBuy(w http.ResponseWriter, r *http.Request, command logging.Command) {
	env.commitOrder(w, r, models.BUY, command)
}

func (env *Env) commitSell(w http.ResponseWriter, r *http.Request, command logging.Command) {
	env.commitOrder(w, r, models.SELL, command)
}

func (env *Env) cancelOrder(w http.ResponseWriter, r *http.Request, orderType models.OrderType, command logging.Command) {
	vars := mux.Vars(r)
	username := vars["username"]
	tdb := env.databases[hash(username)%len(env.databases)]

	res, err := tdb.RemoveLastOrderTypeReservation(username, orderType)
	if err != nil {
		errMsg := fmt.Sprintf("Error deleting last %s reservation.", orderType)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}
	env.respondWithJSON(w, http.StatusOK, res)
	return
}

func (env *Env) cancelSell(w http.ResponseWriter, r *http.Request, command logging.Command) {
	env.cancelOrder(w, r, models.SELL, command)
}

func (env *Env) cancelBuy(w http.ResponseWriter, r *http.Request, command logging.Command) {
	env.cancelOrder(w, r, models.BUY, command)
}

func (env *Env) setBuyAmount(w http.ResponseWriter, r *http.Request, command logging.Command) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]
	trans := vars["trans"]
	tdb := env.databases[hash(username)%len(env.databases)]

	buyAmount, err := strconv.Atoi(vars["amount"])
	if err != nil {
		errMsg := fmt.Sprintf("Invalid amount %s.", vars["amount"])
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	trig, err := tdb.QueryUserTrigger(username, symbol, models.BUY)
	if err != nil && err != sql.ErrNoRows {
		errMsg := fmt.Sprintf("Error querying %s triggers for %s", models.BUY, username)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}
	if err != sql.ErrNoRows {
		errMsg := fmt.Sprintf("Error a %s amount already exists for %s and %s. Please cancel before proceeding.", models.BUY, username, symbol)
		err = errors.New(fmt.Sprintf("Error duplicate %s amount for %s and %s.", models.BUY, username, symbol))
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	balance, err := tdb.QueryUserAvailableBalance(username)
	// check that user exists and has enough money
	if err != nil {
		if err == sql.ErrNoRows {
			errMsg := fmt.Sprintf("Failed to find user %s.", username)
			env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
			return
		}

		errMsg := fmt.Sprintf("Error getting user data for %s.", username)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	if balance < buyAmount {
		errMsg := fmt.Sprintf("User does not have enough money to complete trigger %d < %d.", balance, buyAmount)
		err = errors.New("Error not enough money.")
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	tid, err := tdb.CommitSetOrderTransaction(username, symbol, models.BUY, buyAmount, trans)
	if err != nil {
		errMsg := fmt.Sprintf("Error setting buy amount for %s: %s", username, symbol)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	trig, err = tdb.QueryStockTrigger(tid)
	if err != nil {
		errMsg := fmt.Sprintf("Error trigger %d not found after insert.", tid)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	env.respondWithJSON(w, http.StatusOK, trig)
}

func (env *Env) setSellAmount(w http.ResponseWriter, r *http.Request, command logging.Command) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]
	trans := vars["trans"]
	tdb := env.databases[hash(username)%len(env.databases)]

	sellAmount, err := strconv.Atoi(vars["amount"])
	if err != nil {
		errMsg := fmt.Sprintf("Invalid amount %s.", vars["amount"])
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	availableShares, err := tdb.QueryUserAvailableShares(username, symbol)
	if err != nil {
		errMsg := fmt.Sprintf("Error getting user available shares for %s: %s.", username, symbol)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	trig, err := tdb.QueryUserTrigger(username, symbol, models.SELL)
	if err != nil && err != sql.ErrNoRows {
		errMsg := fmt.Sprintf("Error querying %s triggers for %s", models.BUY, username)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}
	if err != sql.ErrNoRows {
		errMsg := fmt.Sprintf("Error a %s amount already exists for %s and %s. Please cancel before proceeding.", models.SELL, username, symbol)
		err = errors.New(fmt.Sprintf("Error duplicate %s amount for %s and %s.", models.SELL, username, symbol))
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	quote, err := dbutils.QueryQuotePrice(env.quoteCache, env.logger, username, symbol, trans)
	if err != nil {
		errMsg := fmt.Sprintf("Error getting quote from quote server for %s: %s.", username, symbol)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	sellShares := sellAmount / quote

	if availableShares < sellShares {
		errMsg := fmt.Sprintf("User does not have enough stock to complete trigger %d < %d.", availableShares, sellShares)
		err = errors.New("Error not enough stock.")
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	tid, err := tdb.CommitSetOrderTransaction(username, symbol, models.SELL, sellShares, trans)
	if err != nil {
		errMsg := fmt.Sprintf("Error setting %s amount for %s: %s", models.SELL, username, symbol)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	trig, err = tdb.QueryStockTrigger(tid)
	if err != nil {
		errMsg := fmt.Sprintf("Error trigger %d not found after insert.", tid)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	env.respondWithJSON(w, http.StatusOK, trig)
}

func (env *Env) setOrderTrigger(w http.ResponseWriter, r *http.Request, orderType models.OrderType, command logging.Command) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]
	triggerPrice, err := strconv.Atoi(vars["triggerPrice"])
	tdb := env.databases[hash(username)%len(env.databases)]

	if err != nil {
		errMsg := fmt.Sprintf("Invalid amount %s.", vars["triggerPrice"])
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	trig, err := tdb.QueryUserTrigger(username, symbol, orderType)
	if err != nil && err != sql.ErrNoRows {
		errMsg := fmt.Sprintf("Error querying %s triggers for %s", orderType, username)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	if err != sql.ErrNoRows && trig.Executable {
		errMsg := fmt.Sprintf("Error a %s trigger already exists for %s and %s. Please cancel before proceeding.", orderType, username, symbol)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	trig.TriggerPrice = triggerPrice
	trig.Executable = true

	err = tdb.UpdateTrigger(trig)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to update %s trigger for %s and %s", orderType, username, symbol)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	//For err checking consider removing
	trig, err = tdb.QueryStockTrigger(trig.ID)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to query updated %s trigger for %s and %s", orderType, username, symbol)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	env.respondWithJSON(w, http.StatusOK, trig)
}

func (env *Env) setBuyTrigger(w http.ResponseWriter, r *http.Request, command logging.Command) {
	env.setOrderTrigger(w, r, models.BUY, command)
}

func (env *Env) setSellTrigger(w http.ResponseWriter, r *http.Request, command logging.Command) {
	env.setOrderTrigger(w, r, models.SELL, command)
}

func (env *Env) executeTriggerTest(w http.ResponseWriter, r *http.Request, command logging.Command) {
	vars := mux.Vars(r)
	username := vars["username"]
	trans := vars["trans"]
	tdb := env.databases[hash(username)%len(env.databases)]

	rTrigs, err := tdb.QueryAndExecuteCurrentTriggers(env.quoteCache, trans)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to execute triggers for %s.", username)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	env.respondWithJSON(w, http.StatusOK, rTrigs)
}

func (env *Env) cancelTrigger(w http.ResponseWriter, r *http.Request, orderType models.OrderType, command logging.Command) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]
	trans := vars["trans"]
	tdb := env.databases[hash(username)%len(env.databases)]

	trig, err := tdb.QueryUserTrigger(username, symbol, orderType)
	if err != nil {
		if err == sql.ErrNoRows {
			errMsg := fmt.Sprintf("Error no %s trigger exists for %s and %s.", orderType, username, symbol)
			env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
			return
		}
		errMsg := fmt.Sprintf("Error querying %s triggers for %s", orderType, username, command, vars)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	trig, err = tdb.CancelOrderTransaction(trig, trans)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to cancel %s trigger for %s and %s", orderType, username, symbol)
		env.respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	env.respondWithJSON(w, http.StatusOK, trig)
}

func (env *Env) cancelSetBuy(w http.ResponseWriter, r *http.Request, command logging.Command) {
	env.cancelTrigger(w, r, models.BUY, command)
}

func (env *Env) cancelSetSell(w http.ResponseWriter, r *http.Request, command logging.Command) {
	env.cancelTrigger(w, r, models.SELL, command)
}

func (env *Env) dumplog(w http.ResponseWriter, r *http.Request, command logging.Command) {
	vars := mux.Vars(r)
	filename, _ := url.PathUnescape(vars["filename"])
	env.logger.SendDumpLog(filename, "")
	var m map[string]string
	m = make(map[string]string)
	m["filename"] = filename
	env.respondWithJSON(w, http.StatusOK, m)
}

func (env *Env) dumplogUser(w http.ResponseWriter, r *http.Request, command logging.Command) {
	vars := mux.Vars(r)
	username := vars["username"]
	filename, _ := url.PathUnescape(vars["filename"])
	env.logger.SendDumpLog(filename, username)
	var m map[string]string
	m = make(map[string]string)
	m["filename"] = filename
	env.respondWithJSON(w, http.StatusOK, m)
}

func (env *Env) displaySummary(w http.ResponseWriter, r *http.Request, command logging.Command) {
	return
}

func validateURLParams(r *http.Request) (err error) {
	vars := mux.Vars(r)

	username, ok := vars["username"]
	if ok != false {
		if len(username) <= 0 {
			return errors.New("Invalid username\n")
		}
	}

	stock, ok := vars["stock"]
	if ok != false {
		// v, err := strconv.Atoi(stock)
		if len(stock) <= 0 || len(stock) > 3 {
			return errors.New("Invalid stock\n")
		}
		// allows stocks to be numbers
		// could add check for stocks not being number values
	}

	amount, ok := vars["amount"]
	if ok != false {
		floatAmount, err := strconv.ParseFloat(amount, 64)
		if floatAmount <= 0 || err != nil {
			return errors.New("Invalid amount\n")
		}
	}

	money, ok := vars["money"]
	if ok != false {
		floatMoney, err := strconv.ParseFloat(money, 64)
		if floatMoney <= 0 || err != nil {
			return errors.New("Invalid money\n")
		}
	}

	triggerPrice, ok := vars["triggerPrice"]
	if ok != false {
		floatTriggerPrice, err := strconv.ParseFloat(triggerPrice, 64)
		if floatTriggerPrice <= 0 || err != nil {
			return errors.New("Invalid trigger price\n")
		}
	}

	shares, ok := vars["shares"]
	if ok != false {
		intShares, err := strconv.Atoi(shares)
		if intShares <= 0 || err != nil {
			return errors.New("Invalid number of shares\n")
		}
	}

	orderType, ok := vars["orderType"]
	if ok != false {
		if len(orderType) <= 0 {
			return errors.New("Invalid order type\n")
		}
	}

	totalValue, ok := vars["totalValue"]
	if ok != false {
		floatTotalValue, err := strconv.ParseFloat(totalValue, 64)
		if floatTotalValue <= 0 || err != nil {
			return errors.New("Invalid totalValue\n")
		}
	}

	err = nil
	return err
}

func (env *Env) logHandler(fn extendedHandlerFunc, command logging.Command) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		env.logger.LogCommand(command, mux.Vars(r))
		l := fmt.Sprintf("%s - %s%s", r.Method, r.Host, r.URL)
		err := validateURLParams(r)
		if err != nil {
			utils.LogErr(err, "Url params invalid.")
			http.Error(w, err.Error(), http.StatusBadRequest)
			env.logger.LogErrorEvent(command, mux.Vars(r), "URL param validation failed.")
			return
		}

		log.Println(l)
		fn(w, r, command)
	}
}

func main() {
	logger := logging.NewLoggerConnection()
	quoteCache := transdb.NewQuoteCacheConnection()
	defer quoteCache.Close()

	tdb := transdb.NewTransactionDBConnection("transdb", "5432")
	tdb.DB.SetMaxOpenConns(300)
	databases := make(map[int]transdb.TransactionDataStore)

	defer tdb.DB.Close()
	databases[0] = tdb

	env := &Env{quoteCache: quoteCache, logger: logger, tdb: tdb, databases: databases}
	log.SetFlags(0)
	//log.SetOutput(ioutil.Discard)

	router := mux.NewRouter()
	port := os.Getenv("TRANS_PORT")

	go router.HandleFunc("/api/clearUsers", env.logHandler(env.clearUsers, ""))
	go router.HandleFunc("/api/availableBalance/{username}/{trans}", env.logHandler(env.availableBalance, ""))
	go router.HandleFunc("/api/availableShares/{username}/{symbol}/{trans}", env.logHandler(env.availableShares, ""))

	go router.HandleFunc("/api/add/{username}/{money}/{trans}", env.logHandler(env.addUser, logging.ADD))
	go router.HandleFunc("/api/getQuote/{username}/{symbol}/{trans}", env.logHandler(env.getQuoute, logging.QUOTE))

	go router.HandleFunc("/api/buy/{username}/{symbol}/{amount}/{trans}", env.logHandler(env.buyOrder, logging.BUY))
	go router.HandleFunc("/api/commitBuy/{username}/{trans}", env.logHandler(env.commitBuy, logging.COMMIT_BUY))
	go router.HandleFunc("/api/cancelBuy/{username}/{trans}", env.logHandler(env.cancelBuy, logging.CANCEL_BUY))

	go router.HandleFunc("/api/sell/{username}/{symbol}/{amount}/{trans}", env.logHandler(env.sellOrder, logging.SELL))
	go router.HandleFunc("/api/commitSell/{username}/{trans}", env.logHandler(env.commitSell, logging.COMMIT_SELL))
	go router.HandleFunc("/api/cancelSell/{username}/{trans}", env.logHandler(env.cancelSell, logging.CANCEL_SELL))

	go router.HandleFunc("/api/setBuyAmount/{username}/{symbol}/{amount}/{trans}", env.logHandler(env.setBuyAmount, logging.SET_BUY_AMOUNT))
	go router.HandleFunc("/api/setBuyTrigger/{username}/{symbol}/{triggerPrice}/{trans}", env.logHandler(env.setBuyTrigger, logging.SET_BUY_TRIGGER))
	go router.HandleFunc("/api/cancelSetBuy/{username}/{symbol}/{trans}", env.logHandler(env.cancelSetBuy, logging.CANCEL_SET_BUY))

	go router.HandleFunc("/api/setSellAmount/{username}/{symbol}/{amount}/{trans}", env.logHandler(env.setSellAmount, logging.SET_SELL_AMOUNT))
	go router.HandleFunc("/api/cancelSetSell/{username}/{symbol}/{trans}", env.logHandler(env.cancelSetSell, logging.CANCEL_SET_SELL))
	go router.HandleFunc("/api/setSellTrigger/{username}/{symbol}/{triggerPrice}/{trans}", env.logHandler(env.setSellTrigger, logging.SET_SELL_TRIGGER))

	go router.HandleFunc("/api/dumplog/{filename}/{trans}", env.logHandler(env.dumplog, logging.DUMPLOG))
	go router.HandleFunc("/api/dumplog/{filename}/{username}/{trans}", env.logHandler(env.dumplogUser, logging.DUMPLOG))
	go router.HandleFunc("/api/displaySummary/{username}/{trans}", env.logHandler(env.displaySummary, logging.DISPLAY_SUMMARY))

	// router.HandleFunc("/api/executeTriggers/{username}/{trans}", env.logHandler(env.executeTriggerTest, ""))

	http.Handle("/", router)

	log.Println("Running transaction server on port: " + port)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
		panic(err)
	}

}
