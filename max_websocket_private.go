package max_RESTfulAPI

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"log"

	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

func (Mc *MaxClient) PriviateWebsocket(ctx context.Context) {
	var url string = "wss://max-stream.maicoin.com/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Fatal(err)
	}

	subMsg, err := GetMaxSubscribePrivateMessage(Mc.apiKey, Mc.apiSecret)
	if err != nil {
		log.Fatal(errors.New("fail to construct subscribtion message"))
	}

	err = conn.WriteMessage(websocket.TextMessage, subMsg)
	if err != nil {
		log.Fatal(errors.New("fail to subscribe websocket"))
	}
	Mc.WsClient.connMutex.Lock()
	Mc.WsClient.Conn = conn
	Mc.WsClient.connMutex.Unlock()

	Mc.WsOnErrTurn(false)

	// mainloop
mainloop:
	for {
		select {
		case <-ctx.Done():
			Mc.WsOnErrTurn(false)
			Mc.ShutDown()
			return
		default:
			if Mc.WsClient.Conn == nil {
				Mc.WsOnErrTurn(true)
				break mainloop
			}

			msgtype, msg, err := conn.ReadMessage()
			if err != nil {
				LogWarningToDailyLogFile("read:", err, string(msg), msgtype)
				Mc.WsOnErrTurn(true)
				time.Sleep(time.Millisecond * 500)
				break mainloop
			}

			var msgMap map[string]interface{}
			err = json.Unmarshal(msg, &msgMap)
			if err != nil {
				LogWarningToDailyLogFile(err)
				Mc.WsOnErrTurn(true)
				break mainloop
			}

			errh := Mc.handleMaxSocketMsg(msg)
			if errh != nil {
				Mc.WsOnErrTurn(true)
				break mainloop
			}
		} // end select

		// if there is something wrong that the WS should be reconnected.
		if Mc.WsClient.OnErr {
			break
		}
		time.Sleep(time.Millisecond)
	} // end for

	conn.Close()
	Mc.WsClient.Conn.Close()

	// if it is manual work.
	if !Mc.WsClient.OnErr {
		return
	}
	Mc.WsClient.TmpBranch.Lock()
	Mc.WsClient.TmpBranch.Orders = Mc.ReadOrders()
	Mc.WsClient.TmpBranch.Trades = Mc.ReadTrades()
	Mc.WsClient.TmpBranch.Unlock()

	//message := "max websocket reconnecting"
	//LogInfoToDailyLogFile(message)
	Mc.PriviateWebsocket(ctx)
}

func GetMaxSubscribeMessage(product, channel string, symbols []string) ([]byte, error) {
	param := make(map[string]interface{})
	param["action"] = "sub"

	var args []map[string]interface{}
	for _, symbol := range symbols {
		subscriptions := make(map[string]interface{})
		subscriptions["channel"] = channel
		subscriptions["market"] = symbol
		subscriptions["depth"] = 1
		args = append(args, subscriptions)
	}

	param["subscriptions"] = args
	req, err := json.Marshal(param)
	if err != nil {
		return nil, err
	}
	return req, nil
}

// provide private subscribtion message.
func GetMaxSubscribePrivateMessage(apikey, apisecret string) ([]byte, error) {
	// making signature
	h := hmac.New(sha256.New, []byte(apisecret))
	nonce := time.Now().UnixMilli()               // millisecond.
	h.Write([]byte(strconv.FormatInt(nonce, 10))) // int64 to string.
	signature := hex.EncodeToString(h.Sum(nil))

	// prepare authentication message.
	param := make(map[string]interface{})
	param["action"] = "auth"
	param["apiKey"] = apikey
	param["nonce"] = nonce
	param["signature"] = signature
	//param["filters"] = []string{"order", "account"} //"filters": ["order", "trade"] // ignore account update
	param["id"] = "User"

	req, err := json.Marshal(param)
	if err != nil {
		return nil, err
	}
	return req, nil
}

// ##### #####

func (Mc *MaxClient) handleMaxSocketMsg(msg []byte) error {
	var msgMap map[string]interface{}
	err := json.Unmarshal(msg, &msgMap)
	if err != nil {
		LogWarningToDailyLogFile(err)
		return errors.New("fail to unmarshal message")
	}

	event, ok := msgMap["e"]
	if !ok {
		LogWarningToDailyLogFile("there is no event in message")
		return errors.New("fail to obtain message")
	}

	// distribute the msg
	var err2 error
	switch event {
	case "authenticated":
		//LogInfoToDailyLogFile("websocket subscribtion authenticated")
	case "order_snapshot":
		err2 = Mc.parseOrderSnapshotMsg(msgMap)
	case "trade_snapshot":
		err2 = Mc.parseTradeSnapshotMsg(msgMap)
	case "account_snapshot":
		err2 = Mc.parseAccountMsg(msgMap)
	case "order_update":
		err2 = Mc.parseOrderUpdateMsg(msgMap)
	case "trade_update":
		err2 = Mc.parseTradeUpdateMsg(msgMap)
	case "account_update":
		err2 = Mc.parseAccountMsg(msgMap)
	default:
		err2 = errors.New("event not exist")
	}
	if err2 != nil {
		return errors.New("fail to parse message")
	}
	return nil
}

// Order
//	order_snapshot
func (Mc *MaxClient) parseOrderSnapshotMsg(msgMap map[string]interface{}) error {
	snapshotWsOrders := map[int64]WsOrder{}
	jsonbody, _ := json.Marshal(msgMap["o"])
	var wsOrders []WsOrder
	json.Unmarshal(jsonbody, &wsOrders)

	for i := 0; i < len(wsOrders); i++ {
		snapshotWsOrders[wsOrders[i].Id] = wsOrders[i]
	}

	Mc.UpdateOrders(snapshotWsOrders)

	return nil
}

//	order_update
func (Mc *MaxClient) parseOrderUpdateMsg(msgMap map[string]interface{}) error {
	jsonbody, _ := json.Marshal(msgMap["o"])
	var wsOrders []WsOrder
	json.Unmarshal(jsonbody, &wsOrders)

	for i := 0; i < len(wsOrders); i++ {
		if _, ok := Mc.OrdersBranch.Orders[wsOrders[i].Id]; !ok {
			Mc.OrdersBranch.Orders[wsOrders[i].Id] = wsOrders[i]

			Mc.AddOrder(wsOrders[i].Market, wsOrders[i].Side, 1)
			//fmt.Println("new order arrived: ", wsOrders[i])
		} else {
			switch wsOrders[i].State {
			case "cancel":
				/* if wsOrders[i].ExecutedVolume != "0" {
					Mc.FilledOrdersBranch.Filled[wsOrders[i].Id] = wsOrders[i]
				} */
				Mc.AddOrder(wsOrders[i].Market, wsOrders[i].Side, -1)

				Mc.OrdersBranch.Lock()
				delete(Mc.OrdersBranch.Orders, wsOrders[i].Id)
				Mc.OrdersBranch.Unlock()

			case "done":
				//Mc.FilledOrdersBranch.Filled[wsOrders[i].Id] = wsOrders[i]
				Mc.AddOrder(wsOrders[i].Market, wsOrders[i].Side, -1)

				Mc.OrdersBranch.Lock()
				delete(Mc.OrdersBranch.Orders, wsOrders[i].Id)
				Mc.OrdersBranch.Unlock()
			default:
				if _, ok := Mc.OrdersBranch.Orders[wsOrders[i].Id]; !ok {
					Mc.OrdersBranch.Lock()
					Mc.OrdersBranch.Orders[wsOrders[i].Id] = wsOrders[i]
					Mc.OrdersBranch.Unlock()

				}
				//Mc.FilledOrdersBranch.Filled[wsOrders[i].Id] = wsOrders[i]
			}
		}
	}

	return nil
}

// Trade
//	trade_snapshot
func (Mc *MaxClient) parseTradeSnapshotMsg(msgMap map[string]interface{}) error {
	jsonbody, _ := json.Marshal(msgMap["t"])
	var newTrades []Trade
	json.Unmarshal(jsonbody, &newTrades)
	Mc.trackingTrades(newTrades)

	return nil
}

func (Mc *MaxClient) trackingTrades(snapshottrades []Trade) error {
	Mc.WsClient.TmpBranch.Lock()
	oldTrades := Mc.WsClient.TmpBranch.Trades
	Mc.WsClient.TmpBranch.Trades = []Trade{}
	Mc.WsClient.TmpBranch.Unlock()

	if len(oldTrades) == 0 {
		Mc.UpdateTrades(snapshottrades)
		return nil
	}

	untrades := Mc.ReadUnhedgeTrades()

	tradeMap := map[int64]struct{}{}
	oldTrades = append(oldTrades, untrades...)
	for i := 0; i < len(oldTrades); i++ {
		tradeMap[oldTrades[i].Id] = struct{}{}
	}

	untracked, tracked := make([]Trade, 0, 130), make([]Trade, 0, 130)
	for i := 0; i < len(snapshottrades); i++ {
		if _, ok := tradeMap[snapshottrades[i].Id]; !ok {
			untracked = append(untracked, snapshottrades[i])
		} else {
			tracked = append(tracked, snapshottrades[i])
		}
	}

	Mc.UpdateTrades(tracked)
	if len(untracked) > 0 {
		fmt.Println("trade snapshot:", untracked)
		Mc.TradesArrived(untracked)
	}

	return nil
}

//	trade_update
func (Mc *MaxClient) parseTradeUpdateMsg(msgMap map[string]interface{}) error {
	jsonbody, _ := json.Marshal(msgMap["t"])
	var newTrades []Trade
	json.Unmarshal(jsonbody, &newTrades)
	Mc.TradesArrived(newTrades)
	fmt.Println("trade update: ", newTrades)
	return nil
}

// Account
//	account_snapshot and //	account_update
func (Mc *MaxClient) parseAccountMsg(msgMap map[string]interface{}) error {
	Mc.BalanceBranch.Lock()
	defer Mc.BalanceBranch.Unlock()
	switch reflect.TypeOf(msgMap["B"]).Kind() {
	case reflect.Slice:
		s := reflect.ValueOf(msgMap["B"])
		for i := 0; i < s.Len(); i++ {
			wsCurrency := s.Index(i).Interface().(map[string]interface{})
			wsBalance, err := strconv.ParseFloat(wsCurrency["av"].(string), 64)
			if err != nil {
				wsBalance = 0.0
				return errors.New("fail to parse float")

			}

			wsLocked, err := strconv.ParseFloat(wsCurrency["l"].(string), 64)
			if err != nil {
				wsLocked = 0.0
				return errors.New("fail to parse float")
			}
			b := Balance{
				Name:      wsCurrency["cu"].(string),
				Avaliable: wsBalance,
				Locked:    wsLocked,
			}
			Mc.BalanceBranch.Balance[b.Name] = b
		} // end for
	} // end switch

	return nil
}

func (Mc *MaxClient) WsOnErrTurn(b bool) {
	Mc.WsClient.onErrMutex.Lock()
	defer Mc.WsClient.onErrMutex.Unlock()
	Mc.WsClient.OnErr = b
}

// with channel
func (Mc *MaxClient) PriviateWebsocketWithChannel(ctx context.Context, tradeChan chan []Trade) {
	var url string = "wss://max-stream.maicoin.com/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Fatal(err)
	}

	subMsg, err := GetMaxSubscribePrivateMessage(Mc.apiKey, Mc.apiSecret)
	if err != nil {
		log.Fatal(errors.New("fail to construct subscribtion message"))
	}

	err = conn.WriteMessage(websocket.TextMessage, subMsg)
	if err != nil {
		log.Fatal(errors.New("fail to subscribe websocket"))
	}
	Mc.WsClient.connMutex.Lock()
	Mc.WsClient.Conn = conn
	Mc.WsClient.connMutex.Unlock()

	Mc.WsOnErrTurn(false)

	// mainloop
mainloop:
	for {
		select {
		case <-ctx.Done():
			Mc.WsOnErrTurn(false)
			Mc.ShutDown()
			return
		default:
			if Mc.WsClient.Conn == nil {
				Mc.WsOnErrTurn(true)
				break mainloop
			}

			msgtype, msg, err := conn.ReadMessage()
			if err != nil {
				LogWarningToDailyLogFile("PriviateWebsocketWithChannel read:", err, string(msg), msgtype)
				Mc.WsOnErrTurn(true)
				time.Sleep(time.Millisecond * 500)
				break mainloop
			}

			var msgMap map[string]interface{}
			err = json.Unmarshal(msg, &msgMap)
			if err != nil {
				LogWarningToDailyLogFile(err)
				Mc.WsOnErrTurn(true)
				break mainloop
			}

			errh := Mc.handleMaxSocketMsgWithChannel(msg, tradeChan)
			if errh != nil {
				Mc.WsOnErrTurn(true)
				break mainloop
			}
		} // end select

		// if there is something wrong that the WS should be reconnected.
		if Mc.WsClient.OnErr {
			break
		}
		time.Sleep(time.Millisecond)
	} // end for

	Mc.WsClient.Conn.Close()

	// if it is manual work.
	if !Mc.WsClient.OnErr {
		return
	}
	Mc.WsClient.TmpBranch.Lock()
	Mc.WsClient.TmpBranch.Orders = Mc.ReadOrders()
	Mc.WsClient.TmpBranch.Trades = Mc.ReadTrades()
	Mc.WsClient.TmpBranch.Unlock()

	Mc.PriviateWebsocketWithChannel(ctx, tradeChan)
}

func (Mc *MaxClient) handleMaxSocketMsgWithChannel(msg []byte, tradeChan chan []Trade) error {
	var msgMap map[string]interface{}
	err := json.Unmarshal(msg, &msgMap)
	if err != nil {
		LogWarningToDailyLogFile(err)
		return errors.New("fail to unmarshal message")
	}

	event, ok := msgMap["e"]
	if !ok {
		LogWarningToDailyLogFile("there is no event in message")
		return errors.New("fail to obtain message")
	}

	// distribute the msg
	var err2 error
	switch event {
	case "authenticated":
		//LogInfoToDailyLogFile("websocket subscribtion authenticated")
	case "order_snapshot":
		err2 = Mc.parseOrderSnapshotMsg(msgMap)
	case "trade_snapshot":
		err2 = Mc.parseTradeSnapshotMsgWithChannel(msgMap, tradeChan)
	case "account_snapshot":
		err2 = Mc.parseAccountMsg(msgMap)
	case "order_update":
		err2 = Mc.parseOrderUpdateMsg(msgMap)
	case "trade_update":
		err2 = Mc.parseTradeUpdateMsgWithChannel(msgMap, tradeChan)
	case "account_update":
		err2 = Mc.parseAccountMsg(msgMap)
	}
	if err2 != nil {
		return errors.New("fail to parse message")
	}
	return nil
}

func (Mc *MaxClient) parseTradeSnapshotMsgWithChannel(msgMap map[string]interface{}, tradeChan chan []Trade) error {
	jsonbody, _ := json.Marshal(msgMap["t"])
	var snapshottrades []Trade
	json.Unmarshal(jsonbody, &snapshottrades)

	Mc.WsClient.TmpBranch.Lock()
	oldTrades := Mc.WsClient.TmpBranch.Trades
	Mc.WsClient.TmpBranch.Trades = []Trade{}
	Mc.WsClient.TmpBranch.Unlock()

	if len(oldTrades) == 0 {
		Mc.UpdateTrades(snapshottrades)
		return nil
	}

	tradeMap := map[int64]struct{}{}
	for i := 0; i < len(oldTrades); i++ {
		tradeMap[oldTrades[i].Id] = struct{}{}
	}

	untracked := make([]Trade, 0, 130)
	for i := 0; i < len(snapshottrades); i++ {
		if _, ok := tradeMap[snapshottrades[i].Id]; !ok {
			untracked = append(untracked, snapshottrades[i])
		}
	}

	if len(untracked) > 0 {
		tradeChan <- untracked
	}
	Mc.UpdateTrades(snapshottrades)

	return nil
}

//	trade_update
func (Mc *MaxClient) parseTradeUpdateMsgWithChannel(msgMap map[string]interface{}, tradeChan chan []Trade) error {
	jsonbody, _ := json.Marshal(msgMap["t"])
	var newTrades []Trade
	json.Unmarshal(jsonbody, &newTrades)
	if len(newTrades) != 0 {
		tradeChan <- newTrades
		Mc.AddTrades(newTrades)
	}
	return nil
}

// trade report
func (Mc *MaxClient) TradeReportWebsocket(ctx context.Context) {
	var url string = "wss://max-stream.maicoin.com/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Fatal(err)
	}

	subMsg, err := TradeReportSubscribeMessage(Mc.apiKey, Mc.apiSecret)
	if err != nil {
		log.Fatal(errors.New("fail to construct subscribtion message"))
	}

	err = conn.WriteMessage(websocket.TextMessage, subMsg)
	if err != nil {
		log.Fatal(errors.New("fail to subscribe websocket"))
	}
	Mc.WsClient.connMutex.Lock()
	Mc.WsClient.Conn = conn
	Mc.WsClient.connMutex.Unlock()
	Mc.WsOnErrTurn(false)

	// pint it
	go func() {
		for {
			time.Sleep(time.Minute * 2)
			Mc.WsClient.Conn.WriteMessage(websocket.PingMessage, []byte("ping"))
			Mc.WsClient.onErrMutex.Lock()
			onErr := Mc.WsClient.OnErr
			Mc.WsClient.onErrMutex.Unlock()
			if onErr {
				break
			}
		}
	}()

	// mainloop
mainloop:
	for {
		select {
		case <-ctx.Done():
			Mc.WsOnErrTurn(false)
			Mc.ShutDown()
			return
		default:
			if Mc.WsClient.Conn == nil {
				Mc.WsOnErrTurn(true)
				break mainloop
			}

			msgtype, msg, err := conn.ReadMessage()
			if err != nil {
				log.Println("read:", err, string(msg), msgtype)
				Mc.WsOnErrTurn(true)
				time.Sleep(time.Millisecond * 500)
				break mainloop
			}

			errh := Mc.handleTradeReportMsg(msg)
			if errh != nil {
				log.Println(errh)
				Mc.WsOnErrTurn(true)
				break mainloop
			}
		} // end select

		// if there is something wrong that the WS should be reconnected.
		if Mc.WsClient.OnErr {
			break
		}
		time.Sleep(time.Millisecond)
	} // end for

	Mc.WsClient.Conn.Close()

	// if it is manual work.
	if !Mc.WsClient.OnErr {
		return
	}

	Mc.WsClient.TmpBranch.Lock()
	Mc.WsClient.TmpBranch.Orders = Mc.ReadOrders()
	Mc.WsClient.TmpBranch.Trades = Mc.ReadTrades()
	Mc.WsClient.TmpBranch.Unlock()

	time.Sleep(time.Millisecond * 200)

	go Mc.TradeReportWebsocket(ctx)
}

// provide private subscribtion message.
func TradeReportSubscribeMessage(apikey, apisecret string) ([]byte, error) {
	// making signature
	h := hmac.New(sha256.New, []byte(apisecret))
	nonce := time.Now().UnixMilli()               // millisecond.
	h.Write([]byte(strconv.FormatInt(nonce, 10))) // int64 to string.
	signature := hex.EncodeToString(h.Sum(nil))

	// prepare authentication message.
	param := make(map[string]interface{})
	param["action"] = "auth"
	param["apiKey"] = apikey
	param["nonce"] = nonce
	param["signature"] = signature
	param["filters"] = []string{"trade"} //"filters": ["order", "trade"] // ignore account update
	param["id"] = "User"

	req, err := json.Marshal(param)
	if err != nil {
		return nil, err
	}
	return req, nil
}

func (Mc *MaxClient) handleTradeReportMsg(msg []byte) error {
	var msgMap map[string]interface{}
	err := json.Unmarshal(msg, &msgMap)
	if err != nil {
		LogWarningToDailyLogFile(err)
		return errors.New("fail to unmarshal message")
	}

	event, ok := msgMap["e"]
	if !ok {
		LogWarningToDailyLogFile("there is no event in message")
		return errors.New("fail to obtain message")
	}

	// distribute the msg
	var err2 error
	switch event {
	case "authenticated":
		logrus.Info("MAX trade report websocket connected")
	case "trade_snapshot":
		err2 = Mc.parseTradeReportSnapshotMsg(msgMap)
	case "trade_update":
		err2 = Mc.parseTradeReportUpdateMsg(msgMap)
	default:
		err2 = errors.New("event not exist")
	}
	if err2 != nil {
		fmt.Println(event, string(msg))
		return errors.New("fail to parse message")
	}
	return nil
}

func (Mc *MaxClient) parseTradeReportSnapshotMsg(msgMap map[string]interface{}) error {
	jsonbody, _ := json.Marshal(msgMap["t"])
	var newTrades []Trade
	json.Unmarshal(jsonbody, &newTrades)
	Mc.trackingTradeReports(newTrades)

	return nil
}

func (Mc *MaxClient) parseTradeReportUpdateMsg(msgMap map[string]interface{}) error {
	jsonbody, _ := json.Marshal(msgMap["t"])
	var newTrades []Trade
	json.Unmarshal(jsonbody, &newTrades)
	Mc.tradeReportsArrived(newTrades)

	return nil
}

func (Mc *MaxClient) trackingTradeReports(snapshottrades []Trade) error {
	Mc.WsClient.TmpBranch.Lock()
	oldTrades := Mc.WsClient.TmpBranch.Trades
	Mc.WsClient.TmpBranch.Trades = []Trade{}
	Mc.WsClient.TmpBranch.Unlock()

	if len(oldTrades) == 0 {
		Mc.UpdateTrades(snapshottrades)
		return nil
	}

	untrades := Mc.ReadUnhedgeTrades()

	tradeMap := map[int64]struct{}{}
	oldTrades = append(oldTrades, untrades...)
	for i := 0; i < len(oldTrades); i++ {
		tradeMap[oldTrades[i].Id] = struct{}{}
	}

	untracked, tracked := make([]Trade, 0, 130), make([]Trade, 0, 130)
	for i := 0; i < len(snapshottrades); i++ {
		if _, ok := tradeMap[snapshottrades[i].Id]; !ok {
			untracked = append(untracked, snapshottrades[i])
		} else {
			tracked = append(tracked, snapshottrades[i])
		}
	}

	Mc.UpdateTrades(tracked)
	if len(untracked) > 0 {
		fmt.Println("trade snapshot:", untracked)
		Mc.TradesArrived(untracked)
		Mc.tradeReportsArrived(untracked)
	}

	return nil
}

func (Mc *MaxClient) tradeReportsArrived(trades []Trade) {
	Mc.TradeReportBranch.Lock()
	Mc.TradeReportBranch.TradeReports = append(Mc.TradeReportBranch.TradeReports, trades...)
	Mc.TradeReportBranch.Unlock()
	Mc.TradesArrived(trades)
}
