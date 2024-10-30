// internal/dex/raydium/config.go
package raydium

// DefaultPoolConfig с обновленным типом для RaydiumSwapInstructionCode
var DefaultPoolConfig = &Pool{
	// Программы
	AmmProgramID:   "675kPX9MHTjS2zt1qfr1NYHuzeLXfQM9H24wFSUt1Mp8", // ✅ Подтверждено
	SerumProgramID: "srmqPvymJeFKQ4zGQed1GFppgkRHL9kaELCbyksJtPX",  // ✅ Подтверждено

	// AMM конфигурация
	AmmID:           "58oQChx4yWmvKdwLLZzBi4ChoCc2fqCUWBkwMihLYQo2", // ✅ Подтверждено
	AmmAuthority:    "5Q544fKrFoe6tsEbD7S8EmxGTJYAKtTVhAW5Q5pge4j1", // ✅ Обновлено (authority)
	AmmOpenOrders:   "HmiHHzq4Fym9e1D4qzLS6LDDM3tNsCTBPDWHTLZ763jY", // ✅ Обновлено (openOrders)
	AmmTargetOrders: "CZza3Ej4Mc58MnxWA385itCC9jCo3L1D7zc3LKy1bZMR", // ✅ Обновлено (targetOrders)

	// Token Accounts
	PoolCoinTokenAccount: "DQyrAcCrDXQ7NeoqGgDCZwBvWDcYmFCjSb9JtteuvPpz", // ✅ Обновлено (baseVault)
	PoolPcTokenAccount:   "HLmqeL62xR1QoZ1HKKbXRrdN1p3phKpxRMb2VVopvBBz", // ✅ Обновлено (quoteVault)

	// Serum Market
	SerumMarket:           "8BnEgHoWFysVcuFFX7QztDmzuH8r5ZFvyP3sYwn1XTh6", // ✅ Подтверждено
	SerumBids:             "5jWUncPNBMZJ3sTHKmMLszypVkoRK6bfEQMQUHweeQnh", // ✅ Подтверждено
	SerumAsks:             "EaXdHx7x3mdGA38j5RSmKYSXMzAFzzUXCLNBEDXDn1d5", // ✅ Обновлено
	SerumEventQueue:       "8CvwxZ9Db6XbLD46NZwwmVDZZRDy7eydFcAGkXKh9axa", // ✅ Подтверждено
	SerumCoinVaultAccount: "CKxTHwM9fPMRRvZmFnFoqKNd9pQR21c5Aq9bh5h9oghX", // ✅ Обновлено (marketBaseVault)
	SerumPcVaultAccount:   "6A5NHCj1yF6urc9wZNe6Bcjj4LVszQNj5DwAWG97yzMu", // ✅ Обновлено (marketQuoteVault)
	SerumVaultSigner:      "CTz5UMLQm2SRWHzQnU62Pi4yJqbNGjgRBHqqp6oDHfF7", // ✅ Обновлено (marketAuthority)

	// Дополнительные параметры
	RaydiumSwapInstructionCode: 1, // ✅ Не изменилось
}
