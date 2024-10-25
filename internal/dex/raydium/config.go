// internal/dex/raydium/config.go
package raydium

var DefaultPoolConfig = &Pool{
	// Используем новый Standard AMM программный ID
	AmmProgramID:         "CPMMoo8L3F4NbTegBCKVNunggL7H1ZpdTHKxQB5qKP1C",
	AmmID:                "58oQChx4yWmvKdwLLZzBi4ChoCc2fqCUWBkwMihLYQo2",
	AmmAuthority:         "5Q544fKrFoe6tsEbD7S8EmxGTJYAKtTVhAW5Q5pge4j1",
	AmmOpenOrders:        "HRk9CMrpq7Qr6mhLkWGJR19dZ1P7RtUcYP3qHWKKYXAh",
	AmmTargetOrders:      "CZza3Ej4Mc58MnxWA385itCC9jCo3L1D7zc3LKy1bZMR",
	PoolCoinTokenAccount: "DQyrAcCrDXQ7NeoqGgDCZwBvWDcYmFCjSb9JtteuvPpz",
	PoolPcTokenAccount:   "HLmqeL62xR1QoZ1HKKbXRrdN1p3phKpxRMb2VVopvBBz",
	// Serum/OpenBook программный ID
	SerumProgramID: "9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin",
	// SOL-USDC маркет на OpenBook
	SerumMarket:           "HWHvQhFmJB3NUcu1aihKmrKegfVxBEHzwVX6yZCKEsi1",
	SerumBids:             "J7h7mwMjziRrz7w2cdUCcpMkKS8sdNxZsLWgjuHUcV8e",
	SerumAsks:             "4DNBdnTw6wmrK4NmdSTTxs1kEz47yjqLGuoqsMeHvkMF",
	SerumEventQueue:       "8w4n3fcajhgN8TF74j42ehWvbVJnck5cewpjwhRQpyyc",
	SerumCoinVaultAccount: "36c6YqAwyGKQG66XEp2dJc5JqF8UfwUf1Em3UwFZYxUE",
	SerumPcVaultAccount:   "8CFo8bL8mZQK8abbFyypFMwEDd8tVJjHTTojMLgQTUSZ",
	SerumVaultSigner:      "F8Vyqk3unwxkXukZFQeYyGmFfTG3CAX4v24iyrjEYBJV",
	// Raydium свап инструкция осталась та же
	RaydiumSwapInstructionCode: 9,
}
