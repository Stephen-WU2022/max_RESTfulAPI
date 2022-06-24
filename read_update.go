package max_RESTfulAPI

func (Mc *MaxClient) ReadBaseOrderUnit() string {
	Mc.BaseOrderUnitBranch.RLock()
	defer Mc.BaseOrderUnitBranch.RUnlock()
	BaseOrderUnit := Mc.BaseOrderUnitBranch.BaseOrderUnit

	return BaseOrderUnit
}

func (Mc *MaxClient) ReadExchangeInfo() ExchangeInfo {
	Mc.ExchangeInfoBranch.RLock()
	E := Mc.ExchangeInfoBranch.ExInfo
	Mc.ExchangeInfoBranch.RUnlock()
	return E
}

func (Mc *MaxClient) ReadMarkets() []Market {
	Mc.MarketsBranch.RLock()
	defer Mc.MarketsBranch.RUnlock()
	return Mc.MarketsBranch.Markets
}

func (Mc *MaxClient) ReadTrades() []Trade {
	Mc.TradeBranch.RLock()
	defer Mc.TradeBranch.RUnlock()
	return Mc.TradeBranch.Trades
}

func (Mc *MaxClient) TakeUnhedgeTrades() []Trade {
	Mc.TradeBranch.Lock()
	defer Mc.TradeBranch.Unlock()
	unhedgeTrades := Mc.TradeBranch.UnhedgeTrades
	Mc.TradeBranch.Trades = append(Mc.TradeBranch.Trades, unhedgeTrades...)
	if len(Mc.TradeBranch.Trades) > 105 {
		Mc.TradeBranch.Trades = Mc.TradeBranch.Trades[len(Mc.TradeBranch.Trades)-105:]
	}
	Mc.TradeBranch.UnhedgeTrades = []Trade{}
	return unhedgeTrades
}

func (Mc *MaxClient) ReadUnhedgeTrades() []Trade {
	Mc.TradeBranch.RLock()
	defer Mc.TradeBranch.RUnlock()
	return Mc.TradeBranch.UnhedgeTrades
}

func (Mc *MaxClient) AddTrades(trades []Trade) {
	Mc.TradeBranch.Lock()
	defer Mc.TradeBranch.Unlock()
	Mc.TradeBranch.Trades = append(Mc.TradeBranch.Trades, trades...)
}

func (Mc *MaxClient) UpdateTrades(trades []Trade) {
	Mc.TradeBranch.Lock()
	defer Mc.TradeBranch.Unlock()
	Mc.TradeBranch.Trades = trades
}

func (Mc *MaxClient) UpdateUnhedgeTrades(unhedgetrades []Trade) {
	Mc.TradeBranch.Lock()
	defer Mc.TradeBranch.Unlock()
	Mc.TradeBranch.UnhedgeTrades = unhedgetrades
}

func (Mc *MaxClient) TradesArrived(trades []Trade) {
	Mc.TradeBranch.Lock()
	Mc.TradeBranch.UnhedgeTrades = append(Mc.TradeBranch.UnhedgeTrades, trades...)
	Mc.TradeBranch.Unlock()
}
