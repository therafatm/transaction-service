package models

type OrderType string
type CacheQueryType string

const (
	BUY  = OrderType("BUY")
	SELL = OrderType("SELL")
)

const (
	CacheGet = CacheQueryType("get")
	CacheSet = CacheQueryType("set")
)

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Money    int    `json:"money"`
}

type Reservation struct {
	ID       int64     `json:"id"`
	Username string    `json:"username"`
	Symbol   string    `json:"symbol"`
	Order    OrderType `json:"type"`
	Shares   int       `json:"shares"`
	Amount   int       `json:"amount"`
	Time     int64     `json:"time"`
}

type Stock struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Symbol   string `json:"symbol"`
	Shares   int    `json:"shares"`
}

type StockQuote struct {
	Username string         `json:"username"`
	Symbol   string         `json:"symbol"`
	Value    string         `json:"amount"`
	Qtype    CacheQueryType `json:"CacheQueryType"`
}

type Trigger struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Symbol       string    `json:"symbol"`
	Order        OrderType `json:"type"`
	Amount       int       `json:"amount"`
	TriggerPrice int       `json:"triggerprice"`
	Executable   bool      `json:"executable"`
	Time         int64     `json:"time"`
}
