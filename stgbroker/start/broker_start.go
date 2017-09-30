package main

import (
	"git.oschina.net/cloudzone/smartgo/stgbroker"
)

func main() {
	stopChan := make(chan bool, 1)
	stgbroker.Start(stopChan)

	for {
		select {
		case <-stopChan:
			return
		}
	}

}
