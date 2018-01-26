package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
	"encoding/json"

	"transaction_service/queries/actions"
	"transaction_service/queries/utils"
	"transaction_service/queries/models"
	//"transaction_service/triggers/triggermanager"
	"transaction_service/utils"


	"github.com/gorilla/mux"
	// "github.com/phayes/freeport"
	_ "github.com/lib/pq"
)

var db *sql.DB

func connectToDB() *sql.DB {
	var (
		host     = os.Getenv("POSTGRES_HOST")
		user     = os.Getenv("POSTGRES_USER")
		password = os.Getenv("POSTGRES_PASSWORD")
		dbname = os.Getenv("POSTGRES_DB")
	)

	port, err := strconv.Atoi(os.Getenv("DB_PORT"))

	config := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sql.Open("postgres", config)
	utils.CheckErr(err)

	return db
}

func respondWithError(w http.ResponseWriter, code int, err error, message string) {
	utils.LogErr(err)
	fmt.Println(message)
    respondWithJSON(w, code, map[string]string{"error": err.Error(), "message": message})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
    response, _ := json.Marshal(payload)

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    w.Write(response)
}

//TODO: refactor  + test
func getQuoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	body, err := dbutils.QueryQuote(vars["username"], vars["stock"])

	if err != nil {
		w.Write([]byte("Error getting quote.\n"))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write([]byte(body))
}

func clearUsers(w http.ResponseWriter, r *http.Request) {
	err := dbactions.ClearUsers()
	if err != nil {
		errMsg := "Failed to clear users"
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}
	w.WriteHeader(http.StatusOK)
    w.Write([]byte("Cleared users succesfully."))
}


func addUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	moneyStr := vars["money"]
	errMsg := fmt.Sprintf("Failed to add user %s", username)

	money, err := strconv.Atoi(moneyStr)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	user, err := dbutils.QueryUser(username)

	if err != nil && err == sql.ErrNoRows {
		//user no exist
		newUser := models.User{Username: username, Money: money}
		_, err := dbactions.InsertUser(newUser)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err, errMsg)
			return
		}

	} else if err != nil {
		// error
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return 

	}else{
		// user exists
		user.Money += money
		_, err = dbactions.UpdateUser(user)

		if err != nil {
			respondWithError(w, http.StatusInternalServerError, err, errMsg)
			return
		}
	}

	user, err = dbutils.QueryUser(username)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}
	respondWithJSON(w, http.StatusOK, user)
}

func availableBalance(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]

	_, err := dbutils.QueryUser(username)
	if err != nil && err == sql.ErrNoRows {
		errMsg := fmt.Sprintf("No such user %s exists.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}else if err != nil {
		errMsg := fmt.Sprintf("Error retrieving user %s.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	balance, err := dbutils.QueryUserAvailableBalance(username)
	if err != nil {
		errMsg := fmt.Sprintf("Error getting user available balance for %s.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}
	var m map[string]int
	m = make(map[string]int)
	m["balance"] = balance

	respondWithJSON(w, http.StatusOK, m)
}

func availableShares(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]

	_, err := dbutils.QueryUser(username)
	if err != nil && err == sql.ErrNoRows {
		errMsg := fmt.Sprintf("No such user %s exists.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}else if err != nil {
		errMsg := fmt.Sprintf("Error retrieving user %s.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	balance, err := dbutils.QueryUserAvailableShares(username, symbol)
	if err != nil {
		errMsg := fmt.Sprintf("Error getting user available shares for %s: %s.", username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}
	var m map[string]int
	m = make(map[string]int)
	m["shares"] = balance

	respondWithJSON(w, http.StatusOK, m)
}

func buyOrder(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]
	
	buyAmount, err := strconv.Atoi(vars["amount"])
	if err != nil {
		errMsg := fmt.Sprintf("Invalid amount %s.", vars["amount"])
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	balance, err := dbutils.QueryUserAvailableBalance(username)

	// check that user exists and has enough money
	if err != nil {
		if err == sql.ErrNoRows {
			errMsg := fmt.Sprintf("Failed to find user %s.", username)
			respondWithError(w, http.StatusInternalServerError, err, errMsg)
			return
		}

		errMsg := fmt.Sprintf("Error getting user data for %s.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	if balance < buyAmount {
		errMsg := fmt.Sprintf("User does not have enough money to complete order %d < %d.", balance, buyAmount)
		err = errors.New("Error not enough money.")
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	quote, err := dbutils.QueryQuotePrice(username, symbol)
	if err != nil {
		errMsg := fmt.Sprintf("Error getting quote from quote server for %s: %s.", username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	reservation := models.Reservation{ Username: username, Symbol: symbol, Order: models.BUY }
	reservation.Shares = buyAmount / quote
	reservation.Amount = reservation.Shares * quote
	reservation.Time = time.Now().Unix()

	rid, err := dbactions.AddReservation(nil, reservation)
	if err != nil {
		errMsg := "Error setting buy order."
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}
	
	reserv, err := dbutils.QueryReservation(rid)
	if err != nil {
		errMsg := "Error reservation not found after insert."
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	respondWithJSON(w, http.StatusOK, reserv)

	// remove reservation if not bought within 60 seconds
	go dbactions.RemoveOrder(rid, 60)
}

func sellOrder(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]

	sellAmount, err := strconv.Atoi(vars["amount"])
	if err != nil {
		errMsg := fmt.Sprintf("Invalid amount %s.", vars["amount"])
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	quote, err := dbutils.QueryQuotePrice(username, symbol)
	if err != nil {
		errMsg := fmt.Sprintf("Error getting quote from quote server for %s: %s.", username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	sharesToSell := sellAmount / quote

	availableShares, err := dbutils.QueryUserAvailableShares(username, symbol)
	if err != nil {
		errMsg := fmt.Sprintf("Error querying available shares for %s: %s.", username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	if availableShares < sharesToSell {
		errMsg := fmt.Sprintf("User does not have enough shares to complete order %d < %d", availableShares, sharesToSell)
		err = errors.New("Error not enough shares.")
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	reservation := models.Reservation{ Username: username, Symbol: symbol, Order: models.SELL }
	reservation.Shares = sharesToSell
	reservation.Amount = reservation.Shares * quote
	reservation.Time = time.Now().Unix()

	rid, err := dbactions.AddReservation(nil, reservation)
	if err != nil {
		errMsg := "Error setting sell order."
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	reserv, err := dbutils.QueryReservation(rid)
	if err != nil {
		errMsg := "Error reservation not found after insert."
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	respondWithJSON(w, http.StatusOK, reserv)

	// remove reservation if not bought within 60 seconds
	go dbactions.RemoveOrder(rid, 60)
}


func commitOrder(w http.ResponseWriter, r *http.Request, orderType models.OrderType) {
	var requestParams = mux.Vars(r)
	username := requestParams["username"]

	res, err := dbutils.QueryLastReservation(username, orderType)
	if err != nil && err == sql.ErrNoRows {
		errMsg := fmt.Sprintf("No reserved %s order to commit.", orderType) 
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}else if err != nil {
		errMsg := fmt.Sprintf("Error finding last %s reservation.", orderType)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	var balance int
	var amount int

	if orderType == models.BUY{
		balance, err = dbutils.QueryUserAvailableBalance(username)
		amount = res.Amount

	}else{
		balance, err = dbutils.QueryUserAvailableShares(username, res.Symbol)
		amount = res.Shares
	}

	// check that user exists and has enough resources
	if err != nil {
		if err == sql.ErrNoRows {
			errMsg := fmt.Sprintf("Failed to find user %s.", username)
			respondWithError(w, http.StatusInternalServerError, err, errMsg)
			return
		}

		errMsg := fmt.Sprintf("Error getting user data for %s.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	if balance < amount {
		errMsg := fmt.Sprintf("User does not have enough resources to complete order %d < %d.", balance, amount)
		err = errors.New("Error not enough resources.")
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	err = dbactions.CommitBuySellTransaction(res)
	if err != nil {
		errMsg := fmt.Sprintf("Error commiting  %s order.", orderType)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	stock, err := dbutils.QueryUserStock(res.Username, res.Symbol)
	if err != nil {
		errMsg := "Error could not find updated stock."
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
	}

	respondWithJSON(w, http.StatusOK, stock)
	return
}

func commitBuy(w http.ResponseWriter, r *http.Request) {
	commitOrder(w, r, models.BUY)
}

func commitSell(w http.ResponseWriter, r *http.Request) {
	commitOrder(w, r, models.SELL)
}

func cancelOrder(w http.ResponseWriter, r *http.Request, orderType models.OrderType) {
	vars := mux.Vars(r)
	username := vars["username"]
	res, err := dbactions.RemoveLastOrderTypeReservation(username, orderType)
	if err != nil {
		errMsg := fmt.Sprintf("Error deleting last %s reservation.", orderType)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}
	respondWithJSON(w, http.StatusOK, res)
	return
}

func cancelSell(w http.ResponseWriter, r *http.Request) {
	cancelOrder(w, r, models.SELL)
}


func cancelBuy(w http.ResponseWriter, r *http.Request) {
	cancelOrder(w, r, models.BUY)
}

func setBuyAmount(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]

	buyAmount, err := strconv.Atoi(vars["amount"])
	if err != nil {
		errMsg := fmt.Sprintf("Invalid amount %s.", vars["amount"])
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	trig, err := dbutils.QueryUserTrigger(username, symbol, models.BUY)
	if err != nil && err != sql.ErrNoRows {
		errMsg := fmt.Sprintf("Error querying %s triggers for %s", models.BUY, username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return 
	}
	if err != sql.ErrNoRows {
		errMsg := fmt.Sprintf("Error a %s amount already exists for %s and %s. Please cancel before proceeding.", models.BUY, username, symbol)
		err = errors.New(fmt.Sprintf("Error duplicate %s amount for %s and %s.", models.BUY, username, symbol))
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return 
	}

	balance, err := dbutils.QueryUserAvailableBalance(username)
	// check that user exists and has enough money
	if err != nil {
		if err == sql.ErrNoRows {
			errMsg := fmt.Sprintf("Failed to find user %s.", username)
			respondWithError(w, http.StatusInternalServerError, err, errMsg)
			return
		}

		errMsg := fmt.Sprintf("Error getting user data for %s.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	if balance < buyAmount {
		errMsg := fmt.Sprintf("User does not have enough money to complete trigger %d < %d.", balance, buyAmount)
		err = errors.New("Error not enough money.")
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	tid, err := dbactions. CommitSetOrderTransaction(username, symbol, models.BUY, buyAmount)
	if err != nil {
		errMsg := fmt.Sprintf("Error setting buy amount for %s: %s", username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}


	trig, err = dbutils.QueryStockTrigger(tid)
	if err != nil {
		errMsg := fmt.Sprintf("Error trigger %d not found after insert.", tid)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	respondWithJSON(w, http.StatusOK, trig)
}

func setSellAmount(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]

	sellAmount, err := strconv.Atoi(vars["amount"])
	if err != nil {
		errMsg := fmt.Sprintf("Invalid amount %s.", vars["amount"])
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	availableShares, err := dbutils.QueryUserAvailableShares(username, symbol)
	if err != nil {
		errMsg := fmt.Sprintf("Error getting user available shares for %s: %s.", username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	trig, err := dbutils.QueryUserTrigger(username, symbol, models.SELL)
	if err != nil && err != sql.ErrNoRows {
		errMsg := fmt.Sprintf("Error querying %s triggers for %s", models.BUY, username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return 
	}
	if err != sql.ErrNoRows {
		errMsg := fmt.Sprintf("Error a %s amount already exists for %s and %s. Please cancel before proceeding.", models.SELL, username, symbol)
		err = errors.New(fmt.Sprintf("Error duplicate %s amount for %s and %s.", models.SELL, username, symbol))
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return 
	}

	quote, err := dbutils.QueryQuotePrice(username, symbol)
	if err != nil {
		errMsg := fmt.Sprintf("Error getting quote from quote server for %s: %s.", username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	sellShares := sellAmount / quote

	if availableShares < sellShares {
		errMsg := fmt.Sprintf("User does not have enough stock to complete trigger %d < %d.", availableShares, sellShares)
		err = errors.New("Error not enough stock.")
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	tid, err := dbactions.CommitSetOrderTransaction(username, symbol, models.SELL, sellShares)
	if err != nil {
		errMsg := fmt.Sprintf("Error setting %s amount for %s: %s", models.SELL, username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	trig, err = dbutils.QueryStockTrigger(tid)
	if err != nil {
		errMsg := fmt.Sprintf("Error trigger %d not found after insert.", tid)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	respondWithJSON(w, http.StatusOK, trig)
}


func setOrderTrigger(w http.ResponseWriter, r *http.Request, orderType models.OrderType) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]
	triggerPrice, err := strconv.Atoi(vars["triggerPrice"])
	if err != nil {
		errMsg := fmt.Sprintf("Invalid amount %s.", vars["triggerPrice"])
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	trig, err := dbutils.QueryUserTrigger(username, symbol, orderType)
	if err != nil && err != sql.ErrNoRows {
		errMsg := fmt.Sprintf("Error querying %s triggers for %s", orderType, username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return 
	}

	if err != sql.ErrNoRows && trig.Executable {
		errMsg := fmt.Sprintf("Error a %s trigger already exists for %s and %s. Please cancel before proceeding.", orderType, username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return 
	}
		
	trig.TriggerPrice = triggerPrice
	trig.Executable = true

	err = dbactions.UpdateTrigger(trig)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to update %s trigger for %s and %s", orderType, username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return 
	}

	//For err checking consider removing
	trig, err = dbutils.QueryStockTrigger(trig.ID)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to query updated %s trigger for %s and %s", orderType, username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return 
	}

	respondWithJSON(w, http.StatusOK, trig)
}

func setBuyTrigger(w http.ResponseWriter, r *http.Request){
	setOrderTrigger(w, r, models.BUY)
}

func setSellTrigger(w http.ResponseWriter, r *http.Request){
	setOrderTrigger(w, r, models.SELL)
}


func executeTriggerTest(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]

	rTrigs, err := dbactions.QueryAndExecuteCurrentTriggers()
	if err != nil {
		errMsg := fmt.Sprintf("Failed to execute triggers for %s.", username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return 
	}
	respondWithJSON(w, http.StatusOK, rTrigs)
}

func cancelTrigger(w http.ResponseWriter, r *http.Request, orderType models.OrderType) {
	vars := mux.Vars(r)
	username := vars["username"]
	symbol := vars["symbol"]

	trig, err := dbutils.QueryUserTrigger(username, symbol, orderType)
	if err != nil {
		if err == sql.ErrNoRows {
			errMsg := fmt.Sprintf("Error no %s trigger exists for %s and %s.", orderType, username, symbol)
			respondWithError(w, http.StatusInternalServerError, err, errMsg)
			return 
		}
		errMsg := fmt.Sprintf("Error querying %s triggers for %s", orderType, username)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return 
	}

	trig, err = dbactions.CancelOrderTransaction(trig)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to cancel %s trigger for %s and %s", orderType, username, symbol)
		respondWithError(w, http.StatusInternalServerError, err, errMsg)
		return
	}

	respondWithJSON(w, http.StatusOK, trig)
}

func cancelSetBuy(w http.ResponseWriter, r *http.Request) {
	cancelTrigger(w, r, models.BUY)
}

func cancelSetSell(w http.ResponseWriter, r *http.Request) {
	cancelTrigger(w, r, models.SELL)
}


func logHandler(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := fmt.Sprintf("%s - %s%s", r.Method, r.Host, r.URL)
		// err := validateURLParams(r)
		// if err != nil {
		// 	utils.LogErr(err)
		// 	http.Error(w, err.Error(), http.StatusBadRequest)
		// 	return
		// }

		log.Println(l)
		fn(w, r)
	}
}

func main() {
	db = connectToDB()
	defer db.Close()

	dbactions.SetActionsDB(db)
	dbutils.SetUtilsDB(db)

	router := mux.NewRouter()
	port := 8888

	
	router.HandleFunc("/api/clearUsers", logHandler(clearUsers))
	router.HandleFunc("/api/availableBalance/{username}", logHandler(availableBalance))
	router.HandleFunc("/api/availableShares/{username}/{symbol}", logHandler(availableShares))


	router.HandleFunc("/api/add/{username}/{money}", logHandler(addUser))
	router.HandleFunc("/api/getQuote/{username}/{stock}", logHandler(getQuoute))

	router.HandleFunc("/api/buy/{username}/{symbol}/{amount}", logHandler(buyOrder))
	router.HandleFunc("/api/commitBuy/{username}", logHandler(commitBuy))
	router.HandleFunc("/api/cancelBuy/{username}", logHandler(cancelBuy))

	router.HandleFunc("/api/sell/{username}/{symbol}/{amount}", logHandler(sellOrder))
	router.HandleFunc("/api/commitSell/{username}", logHandler(commitSell))
	router.HandleFunc("/api/cancelSell/{username}", logHandler(cancelSell))

	router.HandleFunc("/api/setBuyAmount/{username}/{symbol}/{amount}", logHandler(setBuyAmount))
	router.HandleFunc("/api/setBuyTrigger/{username}/{symbol}/{triggerPrice}", logHandler(setBuyTrigger))
	router.HandleFunc("/api/cancelSetBuy/{username}/{symbol}", logHandler(cancelSetBuy))

	router.HandleFunc("/api/setSellAmount/{username}/{symbol}/{amount}", logHandler(setSellAmount))
	router.HandleFunc("/api/cancelSetSell/{username}/{symbol}", logHandler(cancelSetSell))
	router.HandleFunc("/api/setSellTrigger/{username}/{symbol}/{triggerPrice}", logHandler(setSellTrigger))

	router.HandleFunc("/api/executeTriggers/{username}", logHandler(executeTriggerTest))

	http.Handle("/", router)

	// go triggermanager.Manage()

	if err := http.ListenAndServe(":"+strconv.Itoa(port), nil); err != nil {
		log.Fatal(err)
		panic(err)
	}

	log.Println("Running transaction server on port: " + strconv.Itoa(port))
}
