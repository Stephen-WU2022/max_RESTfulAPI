package max_RESTfulAPI

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	"log"

	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

func (Mc *MaxClient) TradeReportStream(ctx context.Context) {
	go Mc.TradeReportWebsocket(ctx)

	/* // pint it
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:

				time.Sleep(time.Minute * 2)
				Mc.WsClient.connMutex.Lock()
				err := Mc.WsClient.Conn.WriteMessage(websocket.PingMessage, []byte("test"))
				fmt.Println("⭐️", err)
				Mc.WsClient.connMutex.Unlock()
			}
		}
	}() */
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
	Mc.wsOnErrTurn(false)

	Mc.WsClient.Conn.SetPingHandler(nil)

	// mainloop
mainloop:
	for {
		select {
		case <-ctx.Done():
			Mc.wsOnErrTurn(false)
			Mc.ShutDown()
			return
		default:
			if Mc.WsClient.Conn == nil {
				Mc.wsOnErrTurn(true)
				break mainloop
			}

			Mc.WsClient.connMutex.Lock()
			msgtype, msg, err := conn.ReadMessage()
			Mc.WsClient.connMutex.Unlock()
			if err != nil {
				log.Println("read:", err, string(msg), msgtype)
				Mc.wsOnErrTurn(true)
				time.Sleep(time.Millisecond * 500)
				break mainloop
			}

			errh := Mc.handleTradeReportMsg(msg)
			if errh != nil {
				log.Println(errh)
				Mc.wsOnErrTurn(true)
				break mainloop
			}
		} // end select

		// if there is something wrong that the WS should be reconnected.
		if Mc.WsClient.OnErr {
			break
		}
		time.Sleep(time.Millisecond * 50)
	} // end for

	Mc.WsClient.Conn.Close()

	// if it is manual work.
	if !Mc.WsClient.OnErr {
		return
	}

	Mc.WsClient.TmpBranch.Lock()
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
		log.Print("MAX trade report websocket connected")
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

func (Mc *MaxClient) wsOnErrTurn(b bool) {
	Mc.WsClient.onErrMutex.Lock()
	defer Mc.WsClient.onErrMutex.Unlock()
	Mc.WsClient.OnErr = b
}
