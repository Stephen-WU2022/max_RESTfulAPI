package max_RESTfulAPI

import (
	"fmt"
	"github.com/shopspring/decimal"
	"net/http"
)

// MaxMarketInfo represents the data structure of a market.
type MaxMarketInfo struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	MarketStatus       string          `json:"market_status"`
	BaseUnit           string          `json:"base_unit"`
	BaseUnitPrecision  int             `json:"base_unit_precision"`
	MinBaseAmount      decimal.Decimal `json:"min_base_amount"`
	QuoteUnit          string          `json:"quote_unit"`
	QuoteUnitPrecision int             `json:"quote_unit_precision"`
	MinQuoteAmount     decimal.Decimal `json:"min_quote_amount"`
	MWalletSupported   bool            `json:"m_wallet_supported"`
}

// GetMaxMarketInfo fetches market information from the API.
func GetMaxMarketInfo() ([]MaxMarketInfo, error) {
	// Define the API URL.
	apiURL := "https://max-api.maicoin.com/api/v2/markets"

	// Make an HTTP GET request to the API.
	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check the response status code.
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(" Max market info API request failed with status code: %d", resp.StatusCode)
	}

	// Decode the JSON response into a slice of MaxMarketInfo.
	var response []MaxMarketInfo
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, err
	}

	return response, nil
}
