package max_RESTfulAPI

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

func (Mc *MaxClient) GetAccount() (Member, error) {
	member, _, err := Mc.ApiClient.PrivateApi.GetApiV2MembersAccounts(context.Background(), Mc.apiKey, Mc.apiSecret)
	if err != nil {
		return Member{}, errors.New("fail to get account")
	}
	return member, nil
}

// Get balance into a map with key denote asset.
func (Mc *MaxClient) GetBalance() (map[string]Balance, error) {
	localbalance := map[string]Balance{}

	member, _, err := Mc.ApiClient.PrivateApi.GetApiV2MembersAccounts(context.Background(), Mc.apiKey, Mc.apiSecret)
	if err != nil {
		return map[string]Balance{}, err
	}
	Accounts := member.Accounts
	for i := 0; i < len(Accounts); i++ {
		currency := Accounts[i].Currency
		balance, err := strconv.ParseFloat(Accounts[i].Balance, 64)
		if err != nil {

			Mc.logger.Error(err)
			return map[string]Balance{}, errors.New("fail to parse balance to float64")
		}
		locked, err := strconv.ParseFloat(Accounts[i].Locked, 64)
		if err != nil {
			Mc.logger.Error(err)
			return map[string]Balance{}, errors.New("fail to parse locked balance to float64")
		}

		b := Balance{
			Name:      currency,
			Avaliable: balance,
			Locked:    locked,
		}
		localbalance[currency] = b
	}

	return localbalance, nil
}

// Get orders of the coresponding $market
func (Mc *MaxClient) GetOrders(market string) (map[int64]WsOrder, error) {
	orders, _, err := Mc.ApiClient.PrivateApi.GetApiV2Orders(context.Background(), Mc.apiKey, Mc.apiSecret, market, nil)
	if err != nil {
		return map[int64]WsOrder{}, err
	}

	wsOrders := map[int64]WsOrder{}
	for i := 0; i < len(orders); i++ {
		order := WsOrder(orders[i])
		wsOrders[order.Id] = order

	}
	return wsOrders, nil
}

// Get orders of all markets.
func (Mc *MaxClient) GetAllOrders() (map[int64]WsOrder, error) {
	newOrders := map[int64]WsOrder{}

	orders, _, err := Mc.ApiClient.PrivateApi.GetApiV2Orders(context.Background(), Mc.apiKey, Mc.apiSecret, "all", nil)

	if err != nil {
		return map[int64]WsOrder{}, err
	}

	for i := 0; i < len(orders); i++ {
		newOrders[orders[i].Id] = WsOrder(orders[i])
	}

	return newOrders, nil
}

func (Mc *MaxClient) GetMarkets() ([]Market, error) {
	Mc.MarketsBranch.RLock()
	defer Mc.MarketsBranch.RUnlock()
	markets, _, err := Mc.ApiClient.PublicApi.GetApiV2Markets(context.Background())
	if err != nil {
		return []Market{}, err
	}
	Mc.MarketsBranch.Markets = markets
	return markets, nil
}

func (Mc *MaxClient) CancelAllOrders() ([]WsOrder, error) {

	canceledOrders, _, err := Mc.ApiClient.PrivateApi.PostApiV2OrdersClear(context.Background(), Mc.apiKey, Mc.apiSecret, nil)
	if err != nil {
		return []WsOrder{}, errors.New("fail to cancel all orders")
	}
	canceledWsOrders := make([]WsOrder, 0, len(canceledOrders))
	for i := 0; i < len(canceledOrders); i++ {
		canceledWsOrders = append(canceledWsOrders, WsOrder(canceledOrders[i]))
	}

	return canceledWsOrders, nil
}

func (Mc *MaxClient) CancelOrder(market string, id, clientId interface{}) (wsOrder WsOrder, err error) {
	var canceledorder Order
	if clientId == nil && id == nil {
		return WsOrder{}, errors.New("no order found")
	} else if clientId == nil {
		canceledorder, _, err = Mc.ApiClient.PrivateApi.PostApiV2OrderDelete(context.Background(), Mc.apiKey, Mc.apiSecret, id.(int64))
		if err != nil {
			return WsOrder{}, errors.New("fail to cancel order" + strconv.Itoa(int(id.(int64))))
		}
		Mc.logger.Info("Cancel Order ", id, "by CancelOrder func.")
	} else if id == nil {
		canceledorder, _, err = Mc.ApiClient.PrivateApi.PostApiV2OrderDeleteClientId(context.Background(), Mc.apiKey, Mc.apiSecret, clientId.(string))
		if err != nil {
			return WsOrder{}, errors.New("fail to cancel order" + clientId.(string))
		}
		Mc.logger.Info("Cancel Order with client_id ", clientId.(string), "by CancelOrder func.")
	}

	return WsOrder(canceledorder), nil
}

/*
"side" (string) set tp cancel only sell (asks) or buy (bids) orders
"market" (string) specify market like btctwd / ethbtc
both params can be set as nil.
*/
func (Mc *MaxClient) CancelOrders(market, side interface{}) ([]WsOrder, error) {
	params := make(map[string]interface{})
	if market != nil {
		params["market"] = market.(string)
	}
	if side != nil {
		params["side"] = side.(string)
	}
	// bug exist
	canceledOrders, _, err := Mc.ApiClient.PrivateApi.PostApiV2OrdersClear(context.Background(), Mc.apiKey, Mc.apiSecret, params)
	if err != nil {
		fmt.Println(err)
		return []WsOrder{}, err
	}
	canceledWsOrders := make([]WsOrder, 0, len(canceledOrders))

	return canceledWsOrders, nil
}

func (Mc *MaxClient) PlaceLimitOrder(market string, side string, price, volume float64) (WsOrder, error) {
	/* if isEnough := Mc.checkBalanceEnoughLocal(market, side, price, volume); !isEnough {
		return WsOrder{}, errors.New("balance is not enough for trading")
	} */

	params := make(map[string]interface{})
	params["price"] = fmt.Sprint(price)
	params["ord_type"] = "limit"
	vol := fmt.Sprint(volume)

	order, _, err := Mc.ApiClient.PrivateApi.PostApiV2Orders(context.Background(), Mc.apiKey, Mc.apiSecret, market, side, vol, params)
	if err != nil {
		return WsOrder{}, err
	}

	return WsOrder(order), nil
}

func (Mc *MaxClient) PlacePostOnlyOrder(market string, side string, price, volume float64) (WsOrder, error) {
	/* if isEnough := Mc.checkBalanceEnoughLocal(market, side, price, volume); !isEnough {
		return WsOrder{}, errors.New("balance is not enough for trading")
	} */

	params := make(map[string]interface{})
	params["price"] = fmt.Sprint(price)
	params["ord_type"] = "post_only"
	vol := fmt.Sprint(volume)

	order, _, err := Mc.ApiClient.PrivateApi.PostApiV2Orders(context.Background(), Mc.apiKey, Mc.apiSecret, market, side, vol, params)
	if err != nil {
		return WsOrder{}, err
	}

	return WsOrder(order), nil
}

func (Mc *MaxClient) PlaceMarketOrder(market string, side string, volume float64) (WsOrder, error) {
	params := make(map[string]interface{})
	params["ord_type"] = "market"
	vol := fmt.Sprint(volume)
	order, _, err := Mc.ApiClient.PrivateApi.PostApiV2Orders(context.Background(), Mc.apiKey, Mc.apiSecret, market, side, vol, params)
	if err != nil {
		return WsOrder{}, errors.New("fail to place market orders")
	}

	return WsOrder(order), nil
}

// for modularized arbitrage framework
// GetBalances() ([][]string, bool)     // []string{asset, available, total}
func (Mc *MaxClient) GetBalances() (balances [][]string, ok bool) {
	member, _, err := Mc.ApiClient.PrivateApi.GetApiV2MembersAccounts(context.Background(), Mc.apiKey, Mc.apiSecret)
	if err != nil {
		return [][]string{}, false
	}
	Accounts := member.Accounts
	for i := 0; i < len(Accounts); i++ {
		currency := Accounts[i].Currency
		available, err := decimal.NewFromString(Accounts[i].Balance)
		if err != nil {
			available = decimal.Zero
		}
		locked, err := decimal.NewFromString(Accounts[i].Locked)
		if err != nil {
			locked = decimal.Zero
		}

		// handle the case here.
		balances = append(balances, []string{strings.ToUpper(currency), available.String(), (available.Add(locked)).String()})
	}
	return balances, true
}

// GetOpenOrders() ([][]string, bool)   // []string{oid, symbol, product, subaccount, price, qty, side, execType, UnfilledQty}
func (Mc *MaxClient) GetOpenOrders() (openOrders [][]string, ok bool) {
	orders, _, err := Mc.ApiClient.PrivateApi.GetApiV2Orders(context.Background(), Mc.apiKey, Mc.apiSecret, "all", nil)
	if err != nil {
		return [][]string{}, false
	}

	for _, v := range orders {
		// []string{oid, symbol, product, subaccount, price, qty, side, execType, unfilledQty}
		oid := fmt.Sprint(v.Id)
		symbol := v.Market
		product := "spot"
		price := v.Price
		qty := v.Volume
		side := v.Side
		execType := v.OrdType
		unfilledQty := v.RemainingVolume

		openOrder := []string{oid, symbol, product, "", price, qty, side, execType, unfilledQty}

		openOrders = append(openOrders, openOrder)
	}

	return openOrders, true
}

// GetTradeReports() ([][]string, bool) // []string{oid, symbol, product, subaccount, price, qty, side, execType, fee, filledQty, timestamp, isMaker}
func (Mc *MaxClient) GetTradeReports() ([][]string, bool) {
	Mc.TradeReportBranch.Lock()
	trades := Mc.TradeReportBranch.TradeReports
	Mc.TradeReportBranch.TradeReports = []Trade{}
	Mc.TradeReportBranch.Unlock()

	var tradeReports [][]string
	for i := range trades {
		trade := trades[i]
		oid := fmt.Sprint(trade.Oid)
		symbol := trade.Market
		product := "spot"
		subaccount := ""
		price := trade.Price
		qty := trade.Volume
		side := trade.Side
		maker := trade.Maker
		execType := "limit"
		if !maker {
			execType = "market"
		}
		isMaker := "false"
		if trade.Maker {
			isMaker = "true"
		}
		timestamp := strconv.Itoa(int(trade.Timestamp))
		fee := trade.Fee
		filledQty := trade.Volume

		tradeReport := []string{oid, symbol, product, subaccount, price, qty, side, execType, fee, filledQty, timestamp, isMaker}
		tradeReports = append(tradeReports, tradeReport)
	}

	return tradeReports, len(tradeReports) != 0
}
