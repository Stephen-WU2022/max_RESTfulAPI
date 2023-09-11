package max_RESTfulAPI

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
	"time"

	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

func NewMaxClient(ctx context.Context, APIKEY, APISECRET string, logger *logrus.Logger) *MaxClient {
	// api client
	cfg := NewConfiguration()
	apiclient := NewAPIClient(cfg)

	// Get markets []Market
	markets, _, err := apiclient.PublicApi.GetApiV2Markets(ctx)
	if err != nil {
		logger.Error(err)
	}
	m := MaxClient{}
	m.apiKey = APIKEY
	m.apiSecret = APISECRET
	_, cancel := context.WithCancel(ctx)
	m.cancelFunc = &cancel
	m.ShutingBranch.shut = false
	m.ApiClient = apiclient
	m.MarketsBranch.Markets = markets
	m.logger = logger

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
