// internal/dex/raydium/types.go
package raydium

// RaydiumPoolInfo содержит информацию о пуле Raydium
type RaydiumPoolInfo struct {
	AmmProgramID               string // Program ID AMM Raydium
	AmmID                      string // AMM ID пула
	AmmAuthority               string // Авторитет AMM
	AmmOpenOrders              string // Открытые ордера AMM
	AmmTargetOrders            string // Целевые ордера AMM
	PoolCoinTokenAccount       string // Аккаунт токена пула
	PoolPcTokenAccount         string // Аккаунт токена PC пула
	SerumProgramID             string // Program ID Serum DEX
	SerumMarket                string // Рынок Serum
	SerumBids                  string // Заявки на покупку Serum
	SerumAsks                  string // Заявки на продажу Serum
	SerumEventQueue            string // Очередь событий Serum
	SerumCoinVaultAccount      string // Аккаунт хранилища монет
	SerumPcVaultAccount        string // Аккаунт хранилища PC
	SerumVaultSigner           string // Подписант хранилища Serum
	RaydiumSwapInstructionCode uint64 // Код инструкции свапа Raydium
}

func (r *RaydiumPoolInfo) GetProgramID() string {
	return r.AmmProgramID
}

func (r *RaydiumPoolInfo) GetPoolID() string {
	return r.AmmID
}

func (r *RaydiumPoolInfo) GetTokenAccounts() (string, string) {
	return r.PoolCoinTokenAccount, r.PoolPcTokenAccount
}
