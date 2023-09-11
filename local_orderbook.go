package max_RESTfulAPI

import (
	"context"
	"errors"
	"fmt"
	"github.com/shopspring/decimal"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
)

type OrderbookBranch struct {
	connBranch struct {
		conn *websocket.Conn
		sync.Mutex
	}

	onErrBranch struct {
		onErr bool
		sync.RWMutex
	}
	keeper OrderBookKeeper

	Market string

	lastUpdatedTimestampBranch struct {
		timestamp int64
		sync.RWMutex
	}

	logger *log.Logger
}

type bookstruct struct {
	Channcel  string              `json:"c,omitempty"`
	Event     string              `json:"e,omitempty"`
	Market    string              `json:"M,omitempty"`
	Asks      [][]decimal.Decimal `json:"a,omitempty"`
	Bids      [][]decimal.Decimal `json:"b,omitempty"`
	Timestamp int64               `json:"T,omitempty"`
}

func SpotLocalOrderbook(ctx context.Context, symbol string, logger *logrus.Logger) *OrderbookBranch {
	var o OrderbookBranch
	o.Market = strings.ToLower(symbol)
	o.logger = logger

	go o.maintain(ctx, symbol)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(60 * time.Second)
				message := []byte("ping")
				o.wsWriteMsg(websocket.PingMessage, message)
			}
		}
	}()
	return &o
}

func (o *OrderbookBranch) maintain(ctx context.Context, symbol string) {
	o.wsOnErrTurn(false)
	duration := time.Second * 30
	var url string = "wss://max-stream.maicoin.com/ws"

	// wait 5 second, if the hand shake fail, will terminate the dail
	dailCtx, _ := context.WithDeadline(ctx, time.Now().Add(time.Second*5))
	conn, _, err := websocket.DefaultDialer.DialContext(dailCtx, url, nil)
	if err != nil {
		log.Print("❌ local orderbook dial:", err)
		defer o.maintain(ctx, symbol)
		time.Sleep(1 * time.Second)
		return
	}

	o.setConn(conn)

	subMsg, err := maxSubscribeBookMessage(symbol)
	if err != nil {
		log.Print(errors.New("❌ fail to construct subscribtion message"))
		o.wsOnErrTurn(true)
	}

	o.wsWriteMsg(websocket.TextMessage, subMsg)
	if err != nil {
		o.wsOnErrTurn(true)
		log.Print(errors.New("❌ fail to subscribe websocket"))
	}

	time.Sleep(time.Second)

mainloop:
	for {
		select {
		case <-ctx.Done():
			o.Close()
			return
		default:
			if o.isWsOnErr() {
				break mainloop
			}

			_, msg, err := o.connBranch.conn.ReadMessage()

			if err != nil {
				log.Print("❌ orderbook maintain read:", err)
				o.wsOnErrTurn(true)
				time.Sleep(time.Second)
				break mainloop
			}
			o.connBranch.conn.SetReadDeadline(time.Now().Add(duration))

			errh := o.handleMaxBookSocketMsg(msg)
			if errh != nil {
				//log.Println("❌ orderbook maintain handle:", errh)
				o.wsOnErrTurn(true)
				time.Sleep(time.Second)
				break mainloop
			}
		} // end select
		time.Sleep(time.Millisecond)
	} // end for

	o.Close()

	if !o.isWsOnErr() {
		return
	}
	time.Sleep(500 * time.Millisecond)
	o.maintain(ctx, symbol)
}

// default for the depth 10 (max).
func maxSubscribeBookMessage(symbol string) ([]byte, error) {
	param := make(map[string]interface{})
	param["action"] = "sub"

	var args []map[string]interface{}
	subscriptions := make(map[string]interface{})
	subscriptions["channel"] = "book"
	subscriptions["market"] = strings.ToLower(symbol)
	subscriptions["depth"] = 50
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
		return err2
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
	o.lastUpdatedTimestampBranch.Lock()
	if time.Now().UnixMilli()-book.Timestamp > 5000 {
		o.lastUpdatedTimestampBranch.timestamp = book.Timestamp
		wrongTime = true
	} else if book.Timestamp < o.lastUpdatedTimestampBranch.timestamp {
		wrongTime = true
	} else {
		o.lastUpdatedTimestampBranch.timestamp = book.Timestamp
	}
	o.lastUpdatedTimestampBranch.Unlock()

	if wrongTime {
		o.wsOnErrTurn(true)
		return nil
	}

	// update
	o.keeper.handleUpdate(book.Bids, book.Asks)

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
	fmt.Println(book.Bids)
	// snapshot
	if err := o.keeper.handleSnapshot(book.Bids, book.Asks); err != nil {
		return err
	}

	o.lastUpdatedTimestampBranch.Lock()
	defer o.lastUpdatedTimestampBranch.Unlock()
	o.lastUpdatedTimestampBranch.timestamp = book.Timestamp
	return nil
}

func (o *OrderbookBranch) wsOnErrTurn(b bool) {
	o.onErrBranch.Lock()
	defer o.onErrBranch.Unlock()
	o.onErrBranch.onErr = b
}

func (o *OrderbookBranch) isWsOnErr() (onErr bool) {
	o.onErrBranch.RLock()
	defer o.onErrBranch.RUnlock()
	onErr = o.onErrBranch.onErr
	return
}

func (o *OrderbookBranch) setConn(conn *websocket.Conn) {
	o.connBranch.Lock()
	defer o.connBranch.Unlock()
	o.connBranch.conn = conn
}

func (o *OrderbookBranch) Close() {
	o.connBranch.Lock()
	defer o.connBranch.Unlock()
	o.connBranch.conn.Close()
}

func (o *OrderbookBranch) wsWriteMsg(msgType int, data []byte) {
	o.connBranch.Lock()
	defer o.connBranch.Unlock()
	o.connBranch.conn.WriteMessage(msgType, data)
}

func (o *OrderbookBranch) RefreshOrderBook() error {
	return nil
}
