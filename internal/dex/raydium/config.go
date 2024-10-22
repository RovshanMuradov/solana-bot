// internal/dex/raydium/config.go
package raydium

var DefaultPoolConfig = &Pool{
	AmmProgramID:               "YourAmmProgramID",
	AmmID:                      "YourAmmID",
	AmmAuthority:               "YourAmmAuthority",
	AmmOpenOrders:              "YourAmmOpenOrders",
	AmmTargetOrders:            "YourAmmTargetOrders",
	PoolCoinTokenAccount:       "YourPoolCoinTokenAccount",
	PoolPcTokenAccount:         "YourPoolPcTokenAccount",
	SerumProgramID:             "YourSerumProgramID",
	SerumMarket:                "YourSerumMarket",
	SerumBids:                  "YourSerumBids",
	SerumAsks:                  "YourSerumAsks",
	SerumEventQueue:            "YourSerumEventQueue",
	SerumCoinVaultAccount:      "YourSerumCoinVaultAccount",
	SerumPcVaultAccount:        "YourSerumPcVaultAccount",
	SerumVaultSigner:           "YourSerumVaultSigner",
	RaydiumSwapInstructionCode: 123,
}
