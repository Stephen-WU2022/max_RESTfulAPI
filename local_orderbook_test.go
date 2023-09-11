package max_RESTfulAPI

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

func manuallyStop(cancel *context.CancelFunc, ob *OrderbookBranch) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	fmt.Println("Ctrl + C pressed!")
	ob.Close()

	time.Sleep(time.Second)
	(*cancel)()
}

// test SpotLocalOrderbook
func TestSpotLocalOrderbook(t *testing.T) {

	logger := logrus.New()
	ctx, cancel := context.WithCancel(context.Background())
	O := SpotLocalOrderbook(ctx, "usdttwd", logger)
	go func() {
		for {
			time.Sleep(time.Second * 1)
			bids, ok := O.GetBids()
			if !ok {
				fmt.Println("Bids empty")
				continue
			}

			asks, ok := O.GetAsks()
			if !ok {
				fmt.Println("Asks empty")
				continue
			}
			fmt.Println("Bids:", bids[:4])
			fmt.Println("Asks:", asks[:4])
		}
	}()
	manuallyStop(&cancel, O)
}
