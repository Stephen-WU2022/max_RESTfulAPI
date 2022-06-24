package max_RESTfulAPI

import (
	"context"
	"fmt"
	"os"
	"time"

	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

func NewMaxClient(ctx context.Context, APIKEY, APISECRET string) *MaxClient {
	// api client
	cfg := NewConfiguration()
	apiclient := NewAPIClient(cfg)

	// Get markets []Market
	markets, _, err := apiclient.PublicApi.GetApiV2Markets(ctx)
	if err != nil {
		LogFatalToDailyLogFile(err)
	}
	m := MaxClient{}
	m.apiKey = APIKEY
	m.apiSecret = APISECRET
	_, cancel := context.WithCancel(ctx)
	m.cancelFunc = &cancel
	m.ShutingBranch.shut = false
	m.ApiClient = apiclient
	m.MarketsBranch.Markets = markets

	return &m
}

func (Mc *MaxClient) ShutDown() {
	fmt.Println("Shut Down the program")
	Mc.CancelAllOrders()
	(*Mc.cancelFunc)()

	Mc.ShutingBranch.Lock()
	Mc.ShutingBranch.shut = true
	Mc.ShutingBranch.Unlock()
	time.Sleep(3 * time.Second)
	os.Exit(1)
}
