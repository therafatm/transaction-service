package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"./queries/actions"
	"./queries/utils"
	"./utils"

	"github.com/gorilla/mux"
	// "github.com/phayes/freeport"
	_ "github.com/lib/pq"
)

var db *sql.DB

func connectToDB() *sql.DB {
	var (
		host     = "localhost"
		port     = 5432
		user     = os.Getenv("POSTGRES_USER")
		password = os.Getenv("POSTGRES_PASSWORD")
		dbname   = "transactions"
	)

	config := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sql.Open("postgres", config)
	utils.CheckErr(err)

	return db
}

func getQuoute(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	body, err := dbutils.QueryQuote(vars["username"], vars["stock"])

	if err != nil {
		w.Write([]byte("Error getting quote."))
	}

	w.Write([]byte(body))
}

func addUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	username := vars["username"]
	addMoney := vars["money"]

	_, balance, err := dbutils.QueryUser(username)

	if err == sql.ErrNoRows {
		//add new user
		err := dbactions.InsertUser(username, addMoney)
		if err != nil {
			w.Write([]byte("Failed to add user " + username))
		} else {
			w.Write([]byte("Successfully added user " + username))
		}
		return
	}
	//add money to existing user
	addMoneyFloat, err := strconv.ParseFloat(addMoney, 64)
	balance += addMoneyFloat
	balanceString := fmt.Sprintf("%f", balance)

	err = dbactions.UpdateUser(username, balanceString)

	if err != nil {
		w.Write([]byte("Failed to update user " + username))
	} else {
		w.Write([]byte("Successfully added user " + username))
	}

}

func buyOrder(w http.ResponseWriter, r *http.Request) {
	var balance float64
	const orderType string = "buy"

	vars := mux.Vars(r)
	username := vars["username"]
	stock := vars["stock"]
	buyAmount, _ := strconv.ParseFloat(vars["amount"], 64)

	_, balance, err := dbutils.QueryUser(username)

	if err != nil {
		if err == sql.ErrNoRows {
			w.Write([]byte("Invalid user."))
			return
		}
		utils.LogErr(err)
		w.Write([]byte("Error getting user data."))
	}

	if balance < buyAmount {
		w.Write([]byte("Insufficent balance."))
		return
	}

	body, err := dbutils.QueryQuote(username, stock)
	if err != nil {
		utils.LogErr(err)
		if body != nil {
			w.Write([]byte("Error getting stock quote."))
		}
		w.Write([]byte("Error converting quote to string."))
	}

	quote, _ := strconv.ParseFloat(strings.Split(string(body), ",")[0], 64)
	buyUnits := int(buyAmount / quote)

	_, err = dbactions.AddReservation(username, stock, "buy", buyUnits, quote)

	if err != nil {
		utils.LogErr(err)
		w.Write([]byte("Error reserving stock."))
		return
	}

	w.Write([]byte("Buy order placed. You have 60 seconds to confirm your order; otherwise, it will be dropped."))
	go dbactions.RemoveOrder(username, stock, orderType, buyUnits, quote)
}

func commitBuy(w http.ResponseWriter, r *http.Request) {
	const orderType = "buy"
	var symbol string
	var shares int
	var faceValue float64

	vars := mux.Vars(r)
	username := vars["username"]
	symbol, shares, faceValue, err := dbactions.GetLastReservation(username, orderType)

	if err != nil {
		utils.LogErr(err)
		w.Write([]byte("Error retrieving reservation."))
		return
	}

	amount := float64(shares) * faceValue

	tx, err := db.Begin()
	err = dbactions.UpdateUserMoney(tx, username, amount, orderType)
	if err != nil {
		utils.LogErr(err)
		tx.Rollback()
		w.Write([]byte("Error updating user."))
		return
	}

	err = dbactions.UpdateUserStock(tx, username, symbol, shares, orderType)
	if err != nil {
		utils.LogErr(err)
		tx.Rollback()
		w.Write([]byte("Error updating user stock."))
		return
	}

	err = dbactions.RemoveReservation(tx, username, symbol, orderType, shares, faceValue)
	if err != nil {
		utils.LogErr(err)
		tx.Rollback()
		w.Write([]byte("Error updating reservation."))
		return
	}

	err = tx.Commit()
	if err != nil {
		utils.LogErr(err)
		tx.Rollback()
		w.Write([]byte("Error committing transaction."))
		return
	}

	w.Write([]byte("Sucessfully comitted transaction."))
}

func sellOrder(w http.ResponseWriter, r *http.Request) {
	const orderType = "sell"
	var userShares int

	vars := mux.Vars(r)
	username := vars["username"]
	stock := vars["stock"]
	sellAmount, _ := strconv.ParseFloat(vars["amount"], 64)

	_, userShares, err := dbutils.QueryUserStock(username, stock)

	if err != nil {
		if err == sql.ErrNoRows {
			w.Write([]byte("User has no shares of this stock."))
			return
		}
		utils.LogErr(err)
		w.Write([]byte("Error getting user stock data."))
	}

	body, err := dbutils.QueryQuote(username, stock)
	if err != nil {
		utils.LogErr(err)
		if body != nil {
			w.Write([]byte("Error getting stock quote."))
		}
		w.Write([]byte("Error converting quote to string."))
	}

	quote, _ := strconv.ParseFloat(strings.Split(string(body), ",")[0], 64)
	balance := quote * float64(userShares)

	if balance < sellAmount {
		w.Write([]byte("Insufficent balance to sell stock."))
		return
	}

	_, err = dbactions.AddReservation(username, stock, orderType, userShares, quote)

	if err != nil {
		utils.LogErr(err)
		w.Write([]byte("Error reserving stock."))
		return
	}

	w.Write([]byte("Sell order placed. You have 60 seconds to confirm your order; otherwise, it will be dropped."))
	go dbactions.RemoveOrder(username, stock, orderType, userShares, quote)
}

// func setBuyAmount(w http.ResponseWriter, r *http.Request) {
// 	vars := mux.Vars(r)
// 	username := vars["username"]
// 	stock := vars["stock"]
// 	buyAmount, _ := strconv.ParseFloat(vars["amount"], 64)

// }

func main() {
	db = connectToDB()
	defer db.Close()

	dbactions.SetActionsDB(db)
	dbutils.SetUtilsDB(db)

	router := mux.NewRouter()
	// port, _ := freeport.GetFreePort()
	port := 8888

	log.Println("Running transaction server on port: " + strconv.Itoa(port))

	router.HandleFunc("/api/getQuote/{username}/{stock}", getQuoute)
	router.HandleFunc("/api/addUser/{username}/{money}", addUser)
	router.HandleFunc("/api/buyOrder/{username}/{stock}/{amount}", buyOrder)
	router.HandleFunc("/api/commitBuy/{username}", commitBuy)
	router.HandleFunc("/api/sellOrder/{username}/{stock}/{amount}", sellOrder)

	// router.HandleFunc("/api/setBuyAmount/{username}/{stock}/{amount}", setBuyAmount)

	// router.HandleFunc("/articles/{category}/{id:[0-9]+}", ArticleHandler)
	http.Handle("/", router)

	if err := http.ListenAndServe(":"+strconv.Itoa(port), nil); err != nil {
		log.Fatal(err)
		panic(err)
	}
}
