// internal/dex/raydium/config.go
package raydium

// DefaultPoolConfig с обновленным типом для RaydiumSwapInstructionCode
var DefaultPoolConfig = &Pool{
	// Актуальный программный ID Raydium
	AmmProgramID: "675kPX9MHTjS2zt1qfr1NYHuzeLXfQM9H24wFSUt1Mp8",

	// Актуальная конфигурация для SOL-USDC пула
	AmmID:                "58oQChx4yWmvKdwLLZzBi4ChoCc2fqCUWBkwMihLYQo2",
	AmmAuthority:         "3uaZBfHPfmpAHW7dsimC1SnyR61X4bJqQZKWmRSCXJxv",
	AmmOpenOrders:        "4NfmERReGt1QCKey8cH5q4LsBYJoUcsuGg11J8GQFwH8",
	AmmTargetOrders:      "38RJcGjtgd4SKRfY2dcM8Z9LzXQR6cyZeGxvjrRsVGZD",
	PoolCoinTokenAccount: "8spXrXn2EWtNiAHvWZY3EE2f8E1TRDHzFTYyXtNuVFKs",
	PoolPcTokenAccount:   "DuYuU5Y6TEZoMhzwPsYYRFzB5xqF999kXGHUDmBZwJge",

	// OpenBook (бывший Serum) маркет
	SerumProgramID:        "srmqPvymJeFKQ4zGQed1GFppgkRHL9kaELCbyksJtPX",
	SerumMarket:           "8BnEgHoWFysVcuFFX7QztDmzuH8r5ZFvyP3sYwn1XTh6",
	SerumBids:             "5jWUncPNBMZJ3sTHKmMLszypVkoRK6bfEQMQUHweeQnh",
	SerumAsks:             "EaXdHx7x3mdGA38j5RSmKYSXMzAFzzUXCHV5T73Sw8TL",
	SerumEventQueue:       "8CvwxZ9Db6XbLD46NZwwmVDZZRDy7eydFcAGkXKh9axa",
	SerumCoinVaultAccount: "CKxTHwM9fPksGqGd5AHjyGWGbzGkDYjP6ABNYRLvJ1Vz",
	SerumPcVaultAccount:   "PCxN9aXvxtwMYrXk8BgESw3NNkGLwpPM8c6DwByrjgN",
	SerumVaultSigner:      "GXWEpRURaQZ9E62Q23EreTUfBy4hfemXgWFUWcg7YFgv",

	// Правильный код инструкции для свапа (теперь uint8)
	RaydiumSwapInstructionCode: 1,
}
