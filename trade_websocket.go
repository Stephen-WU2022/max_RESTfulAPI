package max_RESTfulAPI

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

// trade stream is build for gratching all trades of market.
type TradeStreamBranch struct {
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

	TradeChan    chan TradeData
	tradesBranch struct {
		Trades []TradeData
		sync.Mutex
	}

	lastUpdatedTimestampBranch struct {
		timestamp int64
		mux       sync.RWMutex
	}
	logger *logrus.Logger
}

func maxSubscribeTradeMessage(symbol string) ([]byte, error) {
	var args []map[string]interface{}
	subscriptions := make(map[string]interface{})
	subscriptions["channel"] = "trade"
	subscriptions["market"] = strings.ToLower(symbol)
	args = append(args, subscriptions)

	param := make(map[string]interface{})
	param["action"] = "sub"
	param["subscriptions"] = args
	req, err := json.Marshal(param)
	if err != nil {
		return nil, err
	}
	return req, nil
}

func (o *TradeStreamBranch) ping(ctx context.Context) {
	go func() {
		for {
			time.Sleep(30 * time.Second)
			select {
			case <-ctx.Done():
				return
			default:
				message := []byte("ping")
				o.ConnBranch.conn.WriteMessage(websocket.TextMessage, message)
			}
		}
	}()
}

func SpotTradeStream(symbol string, logger *logrus.Logger) *TradeStreamBranch {
	var o TradeStreamBranch
	ctx, cancel := context.WithCancel(context.Background())
	o.cancel = &cancel
	o.Market = strings.ToLower(symbol)
	o.TradeChan = make(chan TradeData, 100)
	o.logger = logger
	go o.maintain(ctx, symbol)
	go o.listen(ctx)

	time.Sleep(1 * time.Second)
	go o.ping(ctx)

	return &o
}

func (o *TradeStreamBranch) maintain(ctx context.Context, symbol string) {
	var url string = "wss://max-stream.maicoin.com/ws"

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		o.logger.Error(err)
	}
	//LogInfoToDailyLogFile("Connected:", url)
	o.ConnBranch.conn = conn
	o.onErrBranch.mutex.Lock()
	o.onErrBranch.onErr = false
	o.onErrBranch.mutex.Unlock()

	subMsg, err := maxSubscribeTradeMessage(symbol)
	if err != nil {
		o.logger.Error(errors.New("fail to construct subscribtion message"))
	}
	o.ConnBranch.Lock()
	err = o.ConnBranch.conn.WriteMessage(websocket.TextMessage, subMsg)
	o.ConnBranch.Unlock()
	if err != nil {
		o.logger.Error(errors.New("fail to subscribe websocket"))
	}

	NoErr := true
	for NoErr {
		select {
		case <-ctx.Done():
			o.ConnBranch.Lock()
			o.ConnBranch.conn.Close()
			o.ConnBranch.Unlock()
			return
		default:
			o.ConnBranch.Lock()
			_, msg, err := o.ConnBranch.conn.ReadMessage()
			o.ConnBranch.Unlock()
			if err != nil {
				o.logger.Error("read:", err)
				o.onErrBranch.mutex.Lock()
				o.onErrBranch.onErr = true
				o.onErrBranch.mutex.Unlock()
			}

			var msgMap map[string]interface{}
			err = json.Unmarshal(msg, &msgMap)
			if err != nil {
				o.logger.Error(err)
				o.onErrBranch.mutex.Lock()
				o.onErrBranch.onErr = true
				o.onErrBranch.mutex.Unlock()
			}

			errh := o.handleMaxTradeSocketMsg(msg)
			if errh != nil {
				o.onErrBranch.mutex.Lock()
				o.onErrBranch.onErr = true
				o.onErrBranch.mutex.Unlock()
			}

		} // end select

		// if there is something wrong that the WS should be reconnected.
		if o.onErrBranch.onErr {
			//message := "max websocket reconnecting"
			//LogInfoToDailyLogFile(message)
			NoErr = false
		}
		time.Sleep(time.Millisecond)
	} // end for
	o.ConnBranch.Lock()
	o.ConnBranch.conn.Close()
	o.ConnBranch.Unlock()

	if !o.onErrBranch.onErr {
		return
	}
	o.maintain(ctx, symbol)
}

func (o *TradeStreamBranch) handleMaxTradeSocketMsg(msg []byte) error {
	var msgMap map[string]interface{}
	err := json.Unmarshal(msg, &msgMap)
	if err != nil {
		o.logger.Error(err)
		return errors.New("fail to unmarshal message")
	}

	event, ok := msgMap["e"]
	if !ok {
		o.logger.Error("there is no event in message")
		return errors.New("fail to obtain message")
	}

	// distribute the msg
	var err2 error
	switch event {
	case "subscribed":
		fmt.Println("websocket subscribed")
	case "snapshot":
		//err2 = o.parseOrderbookSnapshotMsg(msgMap)
	case "update":
		err2 = o.parseTradeUpdateMsg(msg)
	}

	if err2 != nil {
		fmt.Println(err2, "err2")
		return errors.New("fail to parse message")
	}
	return nil
}

type TradeData struct {
	Channel string `json:"c"`
	Event   string `json:"e"`
	Market  string `json:"M"`
	Trades  []struct {
		P  string `json:"p"`
		V  string `json:"v"`
		T  int    `json:"T"`
		Tr string `json:"tr"`
	} `json:"t"`
	Timestamp int `json:"T"`
}

func (o *TradeStreamBranch) parseTradeUpdateMsg(msg []byte) error {
	var tradeData TradeData
	json.Unmarshal(msg, &tradeData)

	// extract data
	if tradeData.Channel != "trade" {
		return errors.New("wrong channel")
	}
	if tradeData.Event != "update" {
		return errors.New("wrong event")
	}
	if tradeData.Market != o.Market {
		return errors.New("wrong market")
	}

	o.TradeChan <- tradeData

	o.lastUpdatedTimestampBranch.mux.Lock()
	o.lastUpdatedTimestampBranch.timestamp = int64(tradeData.Timestamp)
	o.lastUpdatedTimestampBranch.mux.Unlock()

	return nil
}

func (o *TradeStreamBranch) listen(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-o.TradeChan:
			trade := <-o.TradeChan
			o.tradesBranch.Lock()
			o.tradesBranch.Trades = append(o.tradesBranch.Trades, trade)
			o.tradesBranch.Unlock()
		default:
			time.Sleep(time.Second)
		}
	}
}

func (o *TradeStreamBranch) GetTrades() []TradeData {
	o.tradesBranch.Lock()
	trades := o.tradesBranch.Trades
	o.tradesBranch.Trades = []TradeData{}
	o.tradesBranch.Unlock()
	return trades
}
