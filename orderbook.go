package max_RESTfulAPI

import (
	"github.com/shopspring/decimal"
	"sort"
	"sync"
)

// OrderBookKeeper is local orderbook
type OrderBookKeeper struct {
	sync.RWMutex
	bids [][]decimal.Decimal
	asks [][]decimal.Decimal
}

// handlesanpshot, clean orderbook and set new data
func (o *OrderBookKeeper) handleSnapshot(bids, asks [][]decimal.Decimal) error {
	o.Lock()
	defer o.Unlock()

	// check slice order if bids[i][0] didn't sort by descending, reverse it
	if len(o.bids) > 1 && o.bids[0][0].LessThan(o.bids[1][0]) {
		for i, j := 0, len(o.bids)-1; i < j; i, j = i+1, j-1 {
			o.bids[i], o.bids[j] = o.bids[j], o.bids[i]
		}
	}

	//check slice order if asks[i][0] didn't sort by ascending, reverse it
	if len(o.asks) > 1 && o.asks[0][0].GreaterThan(o.asks[1][0]) {
		for i, j := 0, len(o.asks)-1; i < j; i, j = i+1, j-1 {
			o.asks[i], o.asks[j] = o.asks[j], o.asks[i]
		}
	}

	o.bids = bids
	o.asks = asks

	return nil
}

// handleUpdate, update orderbook
func (o *OrderBookKeeper) handleUpdate(bids, asks [][]decimal.Decimal) {
	o.Lock()
	defer o.Unlock()

	wg := &sync.WaitGroup{}
	wg.Add(2)

	// update bids
	go func() {
		defer wg.Done()
		for _, bid := range bids {
			o.insertOrder(&o.bids, bid, false)
		}
	}()

	// update asks
	go func() {
		defer wg.Done()
		for _, ask := range asks {
			o.insertOrder(&o.asks, ask, true)
		}
	}()

	wg.Wait()
}

// insertOrder, insert order to orderbook
func (o *OrderBookKeeper) insertOrder(s *[][]decimal.Decimal, value []decimal.Decimal, isAscend bool) {
	// build a function to compare price for sort.Search
	var f func(i int) bool
	if isAscend {
		f = func(i int) bool {
			return (*s)[i][0].GreaterThanOrEqual(value[0])
		}
	} else {
		f = func(i int) bool {
			return (*s)[i][0].LessThanOrEqual(value[0])
		}
	}

	index := sort.Search(len(*s), f)

	// Check if price is already in orderbook
	switch {
	// Price haven't existed in orderbook, insert new price
	case index == len(*s) || !(*s)[index][0].Equal(value[0]):
		// check if value's amount is 0, if it is, return
		if value[1].Equal(decimal.Zero) {
			return
		}
		// insert value to orderbook
		*s = append((*s)[:index], append([][]decimal.Decimal{value}, (*s)[index:]...)...)
		return
	// Price already existed in orderbook, update amount
	// Remove if amount = 0
	case value[1].Equal(decimal.Zero):
		*s = append((*s)[:index], (*s)[index+1:]...)
	// default, update amount
	default:
		(*s)[index][1] = value[1]
	}
}

// Get bids and asks
func (o *OrderBookKeeper) Get() (bids, asks [][]decimal.Decimal) {
	o.RLock()
	defer o.RUnlock()

	bidsCopy := make([][]decimal.Decimal, len(o.bids))
	asksCopy := make([][]decimal.Decimal, len(o.asks))
	copy(bidsCopy, o.bids)
	copy(asksCopy, o.asks)

	return bidsCopy, asksCopy
}

func (ob *OrderbookBranch) GetAsks() (asks [][]decimal.Decimal, ok bool) {
	ob.keeper.RLock()
	defer ob.keeper.RUnlock()

	asks = ob.keeper.asks
	if len(asks) == 0 {
		return [][]decimal.Decimal{}, false
	}

	return asks, true
}

func (ob *OrderbookBranch) GetBids() (bids [][]decimal.Decimal, ok bool) {
	ob.keeper.RLock()
	defer ob.keeper.RUnlock()

	bids = ob.keeper.bids
	if len(bids) == 0 {
		return [][]decimal.Decimal{}, false
	}

	return bids, true
}
