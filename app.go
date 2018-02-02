package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"transaction_service/queries/actions"
	"transaction_service/queries/models"
	"transaction_service/queries/utils"
	//"transaction_service/triggers/triggermanager"
	"transaction_service/logger"
	"transaction_service/utils"

	"github.com/gorilla/mux"
	// "github.com/phayes/freeport"
	_ "github.com/lib/pq"
)

var db *sql.DB

type extendedHandlerFunc func(http.ResponseWriter, *http.Request, logger.Command)

func connectToDB() *sql.DB {
	var (
		host     = os.Getenv("POSTGRES_HOST")
		user     = os.Getenv("POSTGRES_USER")
		password = os.Getenv("POSTGRES_PASSWORD")
		dbname   = os.Getenv("POSTGRES_DB")
		port     = os.Getenv("POSTGRES_PORT")
	)

	config := fmt.Sprintf("host=%s port=%s user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sql.Open("postgres", config)
	if err != nil {
		utils.LogErr(err, "Error connecting to DB.")
		panic(err)
	}

	return db
}

func respondWithError(w http.ResponseWriter, code int, err error, message string, command logger.Command, vars map[string]string) {
	logger.LogErrorEvent(command, vars, message)
	utils.LogErrSkip(err, message, 2) // skip 2 stack frames to get actual caller
	respondWithJSON(w, code, map[string]string{"error": err.Error(), "message": message})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

//TODO: refactor  + test
func getQuoute(w http.ResponseWriter, r *http.Request, command logger.Command) {
	vars := mux.Vars(r)
	price, err := dbutils.QueryQuotePrice(vars["username"], vars["symbol"], vars["trans"])
	if err != nil {
		errMsg := fmt.Sprintf("Error getting quote for %s and %s", vars["username"], vars["symbol"])
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}
	respondWithJSON(w, http.StatusOK, map[string]string{"price": string(price), "symbol": vars["symbol"]})
}

//TODO: refactor
func clearUsers(w http.ResponseWriter, r *http.Request, command logger.Command) {
	vars := mux.Vars(r)
	err := dbactions.ClearUsers()
	if err != nil {
		errMsg := "Failed to clear users"
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Cleared users succesfully."))
}

func addUser(w http.ResponseWriter, r *http.Request, command logger.Command) {
	vars := mux.Vars(r)
	username := vars["username"]
	moneyStr := vars["money"]
	errMsg := fmt.Sprintf("Failed to add user %s", username)

	money, err := strconv.Atoi(moneyStr)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	user, err := dbutils.QueryUser(username)

	if err != nil && err == sql.ErrNoRows {
		//user no exist
		newUser := models.User{Username: username, Money: money}
		_, err := dbactions.InsertUser(newUser)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
			return
		}

	} else if err != nil {
		// error
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return

	} else {
		// user exists
		user.Money += money
		_, err = dbactions.UpdateUser(user)

		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
			return
		}
	}

	user, err = dbutils.QueryUser(username)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	respondWithJSON(w, http.StatusOK, user)
}

func availableBalance(w http.ResponseWriter, r *http.Request, command logger.Command) {
	vars := mux.Vars(r)
	username := vars["username"]

	_, err := dbutils.QueryUser(username)
	if err != nil && err == sql.ErrNoRows {
		errMsg := fmt.Sprintf("No such user %s exists.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	} else if err != nil {
		errMsg := fmt.Sprintf("Error retrieving user %s.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	balance, err := dbutils.QueryUserAvailableBalance(username)
	if err != nil {
		errMsg := fmt.Sprintf("Error getting user available balance for %s.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}
	var m map[string]int
	m = make(map[string]int)
	m["balance"] = balance

	respondWithJSON(w, http.StatusOK, m)
}

func availableShares(w http.ResponseWriter, r *http.Request, command logger.Command) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]

	_, err := dbutils.QueryUser(username)
	if err != nil && err == sql.ErrNoRows {
		errMsg := fmt.Sprintf("No such user %s exists.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	} else if err != nil {
		errMsg := fmt.Sprintf("Error retrieving user %s.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	balance, err := dbutils.QueryUserAvailableShares(username, symbol)
	if err != nil {
		errMsg := fmt.Sprintf("Error getting user available shares for %s: %s.", username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}
	var m map[string]int
	m = make(map[string]int)
	m["shares"] = balance

	respondWithJSON(w, http.StatusOK, m)
}

func buyOrder(w http.ResponseWriter, r *http.Request, command logger.Command) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]
	trans := vars["trans"]

	buyAmount, err := strconv.Atoi(vars["amount"])
	if err != nil {
		errMsg := fmt.Sprintf("Invalid amount %s.", vars["amount"])
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	balance, err := dbutils.QueryUserAvailableBalance(username)

	// check that user exists and has enough money
	if err != nil {
		if err == sql.ErrNoRows {
			errMsg := fmt.Sprintf("Failed to find user %s.", username)
			respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
			return
		}

		errMsg := fmt.Sprintf("Error getting user data for %s.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	if balance < buyAmount {
		errMsg := fmt.Sprintf("User does not have enough money to complete order %d < %d.", balance, buyAmount)
		err = errors.New("Error not enough money.")
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	quote, err := dbutils.QueryQuotePrice(username, symbol, trans)
	if err != nil {
		errMsg := fmt.Sprintf("Error getting quote from quote server for %s: %s.", username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	reservation := models.Reservation{Username: username, Symbol: symbol, Order: models.BUY}
	reservation.Shares = buyAmount / quote
	reservation.Amount = reservation.Shares * quote
	reservation.Time = time.Now().Unix()

	rid, err := dbactions.AddReservation(nil, reservation)
	if err != nil {
		errMsg := "Error setting buy order."
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	reserv, err := dbutils.QueryReservation(rid)
	if err != nil {
		errMsg := "Error reservation not found after insert."
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	respondWithJSON(w, http.StatusOK, reserv)

	// remove reservation if not bought within 60 seconds
	go dbactions.RemoveOrder(rid, 60)
}

func sellOrder(w http.ResponseWriter, r *http.Request, command logger.Command) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]
	trans := vars["trans"]

	sellAmount, err := strconv.Atoi(vars["amount"])
	if err != nil {
		errMsg := fmt.Sprintf("Invalid amount %s.", vars["amount"])
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	quote, err := dbutils.QueryQuotePrice(username, symbol, trans)
	if err != nil {
		errMsg := fmt.Sprintf("Error getting quote from quote server for %s: %s.", username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	sharesToSell := sellAmount / quote

	availableShares, err := dbutils.QueryUserAvailableShares(username, symbol)
	if err != nil {
		errMsg := fmt.Sprintf("Error querying available shares for %s: %s.", username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	if availableShares < sharesToSell {
		errMsg := fmt.Sprintf("User does not have enough shares to complete order %d < %d", availableShares, sharesToSell)
		err = errors.New("Error not enough shares.")
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	reservation := models.Reservation{Username: username, Symbol: symbol, Order: models.SELL}
	reservation.Shares = sharesToSell
	reservation.Amount = reservation.Shares * quote
	reservation.Time = time.Now().Unix()

	rid, err := dbactions.AddReservation(nil, reservation)
	if err != nil {
		errMsg := "Error setting sell order."
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	reserv, err := dbutils.QueryReservation(rid)
	if err != nil {
		errMsg := "Error reservation not found after insert."
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	respondWithJSON(w, http.StatusOK, reserv)

	// remove reservation if not bought within 60 seconds
	go dbactions.RemoveOrder(rid, 60)
}

func commitOrder(w http.ResponseWriter, r *http.Request, orderType models.OrderType, command logger.Command) {
	var vars = mux.Vars(r)
	username := vars["username"]
	trans := vars["trans"]

	res, err := dbutils.QueryLastReservation(username, orderType)
	if err != nil && err == sql.ErrNoRows {
		errMsg := fmt.Sprintf("No reserved %s order to commit.", orderType)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	} else if err != nil {
		errMsg := fmt.Sprintf("Error finding last %s reservation.", orderType)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	var balance int
	var amount int

	if orderType == models.BUY {
		balance, err = dbutils.QueryUserAvailableBalance(username)
		amount = res.Amount

	} else {
		balance, err = dbutils.QueryUserAvailableShares(username, res.Symbol)
		amount = res.Shares
	}

	// check that user exists and has enough resources
	if err != nil {
		if err == sql.ErrNoRows {
			errMsg := fmt.Sprintf("Failed to find user %s.", username)
			respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
			return
		}

		errMsg := fmt.Sprintf("Error getting user data for %s.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	if balance < amount {
		errMsg := fmt.Sprintf("User does not have enough resources to complete order %d < %d.", balance, amount)
		err = errors.New("Error not enough resources.")
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	err = dbactions.CommitBuySellTransaction(res, trans)
	if err != nil {
		errMsg := fmt.Sprintf("Error commiting  %s order.", orderType)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	stock, err := dbutils.QueryUserStock(res.Username, res.Symbol)
	if err != nil {
		errMsg := "Error could not find updated stock."
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
	}

	respondWithJSON(w, http.StatusOK, stock)
	return
}

func commitBuy(w http.ResponseWriter, r *http.Request, command logger.Command) {
	commitOrder(w, r, models.BUY, command)
}

func commitSell(w http.ResponseWriter, r *http.Request, command logger.Command) {
	commitOrder(w, r, models.SELL, command)
}

func cancelOrder(w http.ResponseWriter, r *http.Request, orderType models.OrderType, command logger.Command) {
	vars := mux.Vars(r)
	username := vars["username"]
	res, err := dbactions.RemoveLastOrderTypeReservation(username, orderType)
	if err != nil {
		errMsg := fmt.Sprintf("Error deleting last %s reservation.", orderType)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}
	respondWithJSON(w, http.StatusOK, res)
	return
}

func cancelSell(w http.ResponseWriter, r *http.Request, command logger.Command) {
	cancelOrder(w, r, models.SELL, command)
}

func cancelBuy(w http.ResponseWriter, r *http.Request, command logger.Command) {
	cancelOrder(w, r, models.BUY, command)
}

func setBuyAmount(w http.ResponseWriter, r *http.Request, command logger.Command) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]
	trans := vars["trans"]

	buyAmount, err := strconv.Atoi(vars["amount"])
	if err != nil {
		errMsg := fmt.Sprintf("Invalid amount %s.", vars["amount"])
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	trig, err := dbutils.QueryUserTrigger(username, symbol, models.BUY)
	if err != nil && err != sql.ErrNoRows {
		errMsg := fmt.Sprintf("Error querying %s triggers for %s", models.BUY, username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}
	if err != sql.ErrNoRows {
		errMsg := fmt.Sprintf("Error a %s amount already exists for %s and %s. Please cancel before proceeding.", models.BUY, username, symbol)
		err = errors.New(fmt.Sprintf("Error duplicate %s amount for %s and %s.", models.BUY, username, symbol))
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	balance, err := dbutils.QueryUserAvailableBalance(username)
	// check that user exists and has enough money
	if err != nil {
		if err == sql.ErrNoRows {
			errMsg := fmt.Sprintf("Failed to find user %s.", username)
			respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
			return
		}

		errMsg := fmt.Sprintf("Error getting user data for %s.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	if balance < buyAmount {
		errMsg := fmt.Sprintf("User does not have enough money to complete trigger %d < %d.", balance, buyAmount)
		err = errors.New("Error not enough money.")
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	tid, err := dbactions.CommitSetOrderTransaction(username, symbol, models.BUY, buyAmount, trans)
	if err != nil {
		errMsg := fmt.Sprintf("Error setting buy amount for %s: %s", username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	trig, err = dbutils.QueryStockTrigger(tid)
	if err != nil {
		errMsg := fmt.Sprintf("Error trigger %d not found after insert.", tid)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	respondWithJSON(w, http.StatusOK, trig)
}

func setSellAmount(w http.ResponseWriter, r *http.Request, command logger.Command) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]
	trans := vars["trans"]

	sellAmount, err := strconv.Atoi(vars["amount"])
	if err != nil {
		errMsg := fmt.Sprintf("Invalid amount %s.", vars["amount"])
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	availableShares, err := dbutils.QueryUserAvailableShares(username, symbol)
	if err != nil {
		errMsg := fmt.Sprintf("Error getting user available shares for %s: %s.", username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	trig, err := dbutils.QueryUserTrigger(username, symbol, models.SELL)
	if err != nil && err != sql.ErrNoRows {
		errMsg := fmt.Sprintf("Error querying %s triggers for %s", models.BUY, username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}
	if err != sql.ErrNoRows {
		errMsg := fmt.Sprintf("Error a %s amount already exists for %s and %s. Please cancel before proceeding.", models.SELL, username, symbol)
		err = errors.New(fmt.Sprintf("Error duplicate %s amount for %s and %s.", models.SELL, username, symbol))
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	quote, err := dbutils.QueryQuotePrice(username, symbol, trans)
	if err != nil {
		errMsg := fmt.Sprintf("Error getting quote from quote server for %s: %s.", username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	sellShares := sellAmount / quote

	if availableShares < sellShares {
		errMsg := fmt.Sprintf("User does not have enough stock to complete trigger %d < %d.", availableShares, sellShares)
		err = errors.New("Error not enough stock.")
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	tid, err := dbactions.CommitSetOrderTransaction(username, symbol, models.SELL, sellShares, trans)
	if err != nil {
		errMsg := fmt.Sprintf("Error setting %s amount for %s: %s", models.SELL, username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	trig, err = dbutils.QueryStockTrigger(tid)
	if err != nil {
		errMsg := fmt.Sprintf("Error trigger %d not found after insert.", tid)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	respondWithJSON(w, http.StatusOK, trig)
}

func setOrderTrigger(w http.ResponseWriter, r *http.Request, orderType models.OrderType, command logger.Command) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]
	triggerPrice, err := strconv.Atoi(vars["triggerPrice"])
	if err != nil {
		errMsg := fmt.Sprintf("Invalid amount %s.", vars["triggerPrice"])
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	trig, err := dbutils.QueryUserTrigger(username, symbol, orderType)
	if err != nil && err != sql.ErrNoRows {
		errMsg := fmt.Sprintf("Error querying %s triggers for %s", orderType, username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	if err != sql.ErrNoRows && trig.Executable {
		errMsg := fmt.Sprintf("Error a %s trigger already exists for %s and %s. Please cancel before proceeding.", orderType, username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	trig.TriggerPrice = triggerPrice
	trig.Executable = true

	err = dbactions.UpdateTrigger(trig)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to update %s trigger for %s and %s", orderType, username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	//For err checking consider removing
	trig, err = dbutils.QueryStockTrigger(trig.ID)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to query updated %s trigger for %s and %s", orderType, username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	respondWithJSON(w, http.StatusOK, trig)
}

func setBuyTrigger(w http.ResponseWriter, r *http.Request, command logger.Command) {
	setOrderTrigger(w, r, models.BUY, command)
}

func setSellTrigger(w http.ResponseWriter, r *http.Request, command logger.Command) {
	setOrderTrigger(w, r, models.SELL, command)
}

func executeTriggerTest(w http.ResponseWriter, r *http.Request, command logger.Command) {
	vars := mux.Vars(r)
	username := vars["username"]
	trans := vars["trans"]

	rTrigs, err := dbactions.QueryAndExecuteCurrentTriggers(trans)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to execute triggers for %s.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}
	respondWithJSON(w, http.StatusOK, rTrigs)
}

func cancelTrigger(w http.ResponseWriter, r *http.Request, orderType models.OrderType, command logger.Command) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]
	trans := vars["trans"]

	trig, err := dbutils.QueryUserTrigger(username, symbol, orderType)
	if err != nil {
		if err == sql.ErrNoRows {
			errMsg := fmt.Sprintf("Error no %s trigger exists for %s and %s.", orderType, username, symbol)
			respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
			return
		}
		errMsg := fmt.Sprintf("Error querying %s triggers for %s", orderType, username, command, vars)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	trig, err = dbactions.CancelOrderTransaction(trig, trans)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to cancel %s trigger for %s and %s", orderType, username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg, command, vars)
		return
	}

	respondWithJSON(w, http.StatusOK, trig)
}

func cancelSetBuy(w http.ResponseWriter, r *http.Request, command logger.Command) {
	cancelTrigger(w, r, models.BUY, command)
}

func cancelSetSell(w http.ResponseWriter, r *http.Request, command logger.Command) {
	cancelTrigger(w, r, models.SELL, command)
}

func dumplog(w http.ResponseWriter, r *http.Request, command logger.Command) {
	return
}

func displaySummary(w http.ResponseWriter, r *http.Request, command logger.Command) {
	return
}

func logHandler(fn extendedHandlerFunc, command logger.Command) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger.LogCommand(command, mux.Vars(r))
		l := fmt.Sprintf("%s - %s%s", r.Method, r.Host, r.URL)
		// err := validateURLParams(r)
		// if err != nil {
		// 	utils.LogErr(err)
		// 	http.Error(w, err.Error(), http.StatusBadRequest)
		// 	return
		// }

		log.Println(l)
		fn(w, r, command)
	}
}

func main() {
	db = connectToDB()
	defer db.Close()

	dbactions.SetActionsDB(db)
	dbutils.SetUtilsDB(db)

	logger.InitLogger()

	router := mux.NewRouter()
	port := os.Getenv("TRANS_PORT")

	router.HandleFunc("/api/clearUsers", logHandler(clearUsers, ""))
	router.HandleFunc("/api/availableBalance/{username}/{trans}", logHandler(availableBalance, ""))
	router.HandleFunc("/api/availableShares/{username}/{symbol}/{trans}", logHandler(availableShares, ""))

	router.HandleFunc("/api/add/{username}/{money}/{trans}", logHandler(addUser, logger.ADD))
	router.HandleFunc("/api/getQuote/{username}/{symbol}/{trans}", logHandler(getQuoute, logger.QUOTE))

	router.HandleFunc("/api/buy/{username}/{symbol}/{amount}/{trans}", logHandler(buyOrder, logger.BUY))
	router.HandleFunc("/api/commitBuy/{username}/{trans}", logHandler(commitBuy, logger.COMMIT_BUY))
	router.HandleFunc("/api/cancelBuy/{username}/{trans}", logHandler(cancelBuy, logger.CANCEL_BUY))

	router.HandleFunc("/api/sell/{username}/{symbol}/{amount}/{trans}", logHandler(sellOrder, logger.SELL))
	router.HandleFunc("/api/commitSell/{username}/{trans}", logHandler(commitSell, logger.COMMIT_SELL))
	router.HandleFunc("/api/cancelSell/{username}/{trans}", logHandler(cancelSell, logger.CANCEL_SELL))

	router.HandleFunc("/api/setBuyAmount/{username}/{symbol}/{amount}/{trans}", logHandler(setBuyAmount, logger.SET_BUY_AMOUNT))
	router.HandleFunc("/api/setBuyTrigger/{username}/{symbol}/{triggerPrice}/{trans}", logHandler(setBuyTrigger, logger.SET_BUY_TRIGGER))
	router.HandleFunc("/api/cancelSetBuy/{username}/{symbol}/{trans}", logHandler(cancelSetBuy, logger.CANCEL_SET_BUY))

	router.HandleFunc("/api/setSellAmount/{username}/{symbol}/{amount}/{trans}", logHandler(setSellAmount, logger.SET_SELL_AMOUNT))
	router.HandleFunc("/api/cancelSetSell/{username}/{symbol}/{trans}", logHandler(cancelSetSell, logger.CANCEL_SET_SELL))
	router.HandleFunc("/api/setSellTrigger/{username}/{symbol}/{triggerPrice}/{trans}", logHandler(setSellTrigger, logger.SET_SELL_TRIGGER))

	router.HandleFunc("/api/dumplog/{filename}/{trans}", logHandler(dumplog, logger.DUMPLOG))
	router.HandleFunc("/api/displaySummary/{username}/{trans}", logHandler(displaySummary, logger.DISPLAY_SUMMARY))

	router.HandleFunc("/api/executeTriggers/{username}/{trans}", logHandler(executeTriggerTest, ""))

	http.Handle("/", router)

	// go triggermanager.Manage()

	log.Println("Running transaction server on port: " + port)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
		panic(err)
	}

}
