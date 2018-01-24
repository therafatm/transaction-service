package models

type OrderType string

const (
	BUY = OrderType("BUY")
	SELL = OrderType("SELL")
)

type User struct {
	ID int					`json:"id"`
	Username string			`json:"username"`				
	Money int				`json:"money"`
}

type Reservation struct {
	ID int 					`json:"id"`
	Username string			`json:"username"`
	Symbol string			`json:"symbol"`
	Order  OrderType		`json:"type"`
	Shares int 				`json:"shares"`
	Amount int				`json:"amount"`
	Time int64 				`json:"time"`
}

