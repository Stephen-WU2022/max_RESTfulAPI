package max_RESTfulAPI

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Bo-Hao/mapbook"
	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

type OrderbookBranch struct {
	cancel     *context.CancelFunc
	ConnBranch struct {
		conn *websocket.Conn
		sync.Mutex
	}

	onErrBranch struct {
		onErr bool
		mutex sync.RWMutex
	}
	Market string

	bids                       mapbook.BidBook
	asks                       mapbook.AskBook
	lastUpdatedTimestampBranch struct {
		timestamp int64
		mux       sync.RWMutex
	}
}

type bookstruct struct {
	Channcel  string     `json:"c,omitempty"`
	Event     string     `json:"e,omitempty"`
	Market    string     `json:"M,omitempty"`
	Asks      [][]string `json:"a,omitempty"`
	Bids      [][]string `json:"b,omitempty"`
	Timestamp int64      `json:"T,omitempty"`
}

func SpotLocalOrderbook(symbol string, logger *logrus.Logger) *OrderbookBranch {
	var o OrderbookBranch
	ctx, cancel := context.WithCancel(context.Background())
	o.cancel = &cancel
	o.Market = strings.ToLower(symbol)
	o.asks = *mapbook.NewAskBook(false)
	o.bids = *mapbook.NewBidBook(false)
	go o.maintain(ctx, symbol)

	go func() {
		for {
			time.Sleep(60 * time.Second)
			select {
			case <-ctx.Done():
				return
			default:
				message := []byte("ping")

				o.ConnBranch.Lock()
				o.ConnBranch.conn.WriteMessage(websocket.PingMessage, message)
				o.ConnBranch.Unlock()
			}
		}
	}()
	return &o
}

func (o *OrderbookBranch) maintain(ctx context.Context, symbol string) {
	var url string = "wss://max-stream.maicoin.com/ws"

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Print(err)
	}
	//LogInfoToDailyLogFile("Connected:", url)
	o.ConnBranch.conn = conn
	o.onErrBranch.mutex.Lock()
	o.onErrBranch.onErr = false
	o.onErrBranch.mutex.Unlock()

	subMsg, err := maxSubscribeBookMessage(symbol)
	if err != nil {
		log.Print(errors.New("fail to construct subscribtion message"))
	}

	err = conn.WriteMessage(websocket.TextMessage, subMsg)
	if err != nil {
		log.Print(errors.New("fail to subscribe websocket"))
	}
	time.Sleep(time.Second)

	for {
		select {
		case <-ctx.Done():
			o.ConnBranch.Lock()
			o.ConnBranch.conn.Close()
			o.ConnBranch.Unlock()
			return
		default:
			o.onErrBranch.mutex.Lock()
			onErr := o.onErrBranch.onErr
			o.onErrBranch.mutex.Unlock()
			if onErr {
				break
			}

			o.ConnBranch.Lock()
			_, msg, err := o.ConnBranch.conn.ReadMessage()
			o.ConnBranch.Unlock()
			if err != nil {
				log.Print("orderbook maintain read:", err)
				o.onErrBranch.mutex.Lock()
				o.onErrBranch.onErr = true
				o.onErrBranch.mutex.Unlock()
				time.Sleep(time.Second)
				break
			}

			errh := o.handleMaxBookSocketMsg(msg)
			if errh != nil {
				log.Println("orderbook maintain handle:", errh)
				o.onErrBranch.mutex.Lock()
				o.onErrBranch.onErr = true
				o.onErrBranch.mutex.Unlock()
				time.Sleep(time.Second)
			}

		} // end select

		// if there is something wrong that the WS should be reconnected.
		if o.onErrBranch.onErr {
			break
		}
		time.Sleep(time.Millisecond)
	} // end for

	o.ConnBranch.Lock()
	o.ConnBranch.conn.Close()
	o.ConnBranch.Unlock()

	if !o.onErrBranch.onErr {
		return
	}
	go o.maintain(ctx, symbol)
}

// default for the depth 10 (max).
func maxSubscribeBookMessage(symbol string) ([]byte, error) {
	param := make(map[string]interface{})
	param["action"] = "sub"

	var args []map[string]interface{}
	subscriptions := make(map[string]interface{})
	subscriptions["channel"] = "book"
	subscriptions["market"] = strings.ToLower(symbol)
	subscriptions["depth"] = 10
	args = append(args, subscriptions)

	param["subscriptions"] = args
	req, err := json.Marshal(param)
	if err != nil {
		return nil, err
	}
	return req, nil
}

func (o *OrderbookBranch) handleMaxBookSocketMsg(msg []byte) error {
	var msgMap map[string]interface{}
	err := json.Unmarshal(msg, &msgMap)
	if err != nil {
		log.Print(err)
		return errors.New("fail to unmarshal message")
	}

	event, ok := msgMap["e"]
	if !ok {
		log.Print("there is no event in message")
		return errors.New("fail to obtain message")
	}

	// distribute the msg
	var err2 error
	switch event {
	case "subscribed":
		log.Println("✅ Max", o.Market, "orderbook websocket connected.")
	case "snapshot":
		err2 = o.parseOrderbookSnapshotMsg(msgMap)
	case "update":
		err2 = o.parseOrderbookUpdateMsg(msgMap)
	}

	if err2 != nil {
		fmt.Println(err2, "err2")
		return errors.New("fail to parse message")
	}
	return nil
}

func (o *OrderbookBranch) parseOrderbookUpdateMsg(msgMap map[string]interface{}) error {
	jsonbody, _ := json.Marshal(msgMap)
	var book bookstruct
	json.Unmarshal(jsonbody, &book)

	// extract data
	if book.Channcel != "book" {
		return errors.New("wrong channel")
	}
	if book.Event != "update" {
		return errors.New("wrong event")
	}
	if book.Market != o.Market {
		return errors.New("wrong market")
	}

	wrongTime := false
	o.lastUpdatedTimestampBranch.mux.Lock()
	if time.Now().UnixMilli()-book.Timestamp > 5000 {
		o.lastUpdatedTimestampBranch.timestamp = book.Timestamp
		wrongTime = true
	} else if book.Timestamp < o.lastUpdatedTimestampBranch.timestamp {
		wrongTime = true
	} else {
		o.lastUpdatedTimestampBranch.timestamp = book.Timestamp
	}
	o.lastUpdatedTimestampBranch.mux.Unlock()

	if wrongTime {
		o.onErrBranch.mutex.Lock()
		o.onErrBranch.onErr = true
		o.onErrBranch.mutex.Unlock()
		return nil
	}

	// update
	o.asks.Update(book.Asks)
	o.bids.Update(book.Bids)

	return nil
}

func (o *OrderbookBranch) parseOrderbookSnapshotMsg(msgMap map[string]interface{}) error {
	jsonbody, _ := json.Marshal(msgMap)
	var book bookstruct
	json.Unmarshal(jsonbody, &book)

	// extract data
	if book.Channcel != "book" {
		return errors.New("wrong channel")
	}
	if book.Event != "snapshot" {
		fmt.Println("event:", book.Event)
		return errors.New("wrong event")
	}
	if book.Market != o.Market {
		return errors.New("wrong market")
	}

	o.asks.Snapshot(book.Asks)
	o.bids.Snapshot(book.Bids)

	o.lastUpdatedTimestampBranch.mux.Lock()
	if time.Now().UnixMilli()-book.Timestamp > 5000 {
		o.onErrBranch.mutex.Lock()
		o.onErrBranch.onErr = true
		o.onErrBranch.mutex.Unlock()
	}
	o.lastUpdatedTimestampBranch.timestamp = book.Timestamp

	o.lastUpdatedTimestampBranch.mux.Unlock()

	return nil
}

func (o *OrderbookBranch) GetBids() ([][]string, bool) {
	return o.bids.GetAll()
}

func (o *OrderbookBranch) GetAsks() ([][]string, bool) {
	return o.asks.GetAll()
}

func (o *OrderbookBranch) Close() {
	o.ConnBranch.Lock()
	o.ConnBranch.conn.Close()
	o.ConnBranch.Unlock()
	(*o.cancel)()
}
