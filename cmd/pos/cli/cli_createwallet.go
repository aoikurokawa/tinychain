package cli

import (
	"fmt"

	"github.com/Aoi1011/tinychain/pos"
)

func CreateWallet() {
	wallets, _ := pos.NewWallets()
	address := wallets.CreateWallet()
	wallets.SaveToFile()

	fmt.Printf("Your new address: %s\n", address)
}