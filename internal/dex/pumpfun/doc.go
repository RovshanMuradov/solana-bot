// Package pumpfun implements a client for interacting with the Pump.fun decentralized exchange on the Solana blockchain.
//
// This package provides methods for:
// - Buying and selling tokens using exact SOL or token amounts.
// - Fetching and interpreting bonding curve account data.
// - Calculating token prices and balances.
// - Calculating profit and loss (PnL) based on discrete price tiers.
//
// Key Types and Functions:
//
// - DEX struct: Main client struct containing methods for token trading.
// - NewDEX(): Initializes the DEX client instance with proper configuration and blockchain connection.
// - Config struct: Holds configuration details such as contract addresses, token mint addresses, and monitoring intervals.
// - FetchGlobalAccount(): Retrieves global account data including fee recipients and authorities.
// - ExecuteSnipe(), ExecuteSell(), SellPercentTokens(): Methods to perform token buy and sell operations.
// - CalculateDiscretePnL(): Calculates realistic PnL estimates for discrete price movements on Pump.fun.
//
// Detailed information about each function can be found in their respective source files:
//   - pumpfun.go: Main trading operations (ExecuteSnipe, ExecuteSell).
//   - accounts.go: Fetching and parsing account data from Solana blockchain.
//   - config.go: Configuration and setup utilities.
//   - discrete_pnl.go: Profit and loss calculations with discrete price tiers.
//   - instructions.go: Creating buy/sell instructions for blockchain transactions.
//   - trade.go: Preparation of buy/sell transactions, calculating slippage.
//   - transactions.go: Sending transactions and handling blockchain interaction logic.
//   - types.go: Core types like GlobalAccount and BondingCurve.
//
// Usage example:
//
//	cfg := pumpfun.GetDefaultConfig()
//	err := cfg.SetupForToken("TOKEN_MINT_ADDRESS", logger)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	client, err := solbc.NewClient(rpcUrl)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	dex, err := pumpfun.NewDEX(client, wallet, logger, cfg, "")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	err = dex.ExecuteSnipe(context.Background(), 1.0, 0.5, "0.001", 500000)
//	if err != nil {
//	    log.Fatal(err)
//	}
package pumpfun
