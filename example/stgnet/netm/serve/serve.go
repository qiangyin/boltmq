package main

import (
	"fmt"
	"net"

	"git.oschina.net/cloudzone/smartgo/stgnet/netm"
)

func main() {
	b := netm.NewBootstrap()
	b.SetKeepAlive(false).Bind("0.0.0.0", 8000).
		RegisterHandler(func(buffer []byte, addr string, conn net.Conn) {
			fmt.Println("rece:", string(buffer))
			conn.Write([]byte("hi, client"))
		}).Sync()
}
