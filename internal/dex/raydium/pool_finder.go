// ========================================
// File: internal/dex/raydium/pool_finder.go
// ========================================
package raydium

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"go.uber.org/zap"
)

// Предполагаемые константы, соответствующие структуре пул-аккаунта Raydium AMM V4.
const (
	PoolAccountSize = 388
	BaseMintOffset  = 8
	QuoteMintOffset = 40
)

// RaydiumV4ProgramID – адрес программы Raydium AMM V4.
// Здесь вы можете задать нужное значение, либо передавать его через конфиг.
var RaydiumV4ProgramID = solana.MustPublicKeyFromBase58("675kPX9MHTjS2zt1qfr1NYHuzeLXfQM9H24wFSUt1Mp8")

// PoolData хранит информацию о пуле Raydium.
type PoolData struct {
	PoolAddress solana.PublicKey
	BaseMint    solana.PublicKey
	QuoteMint   solana.PublicKey
}

// DexScreenerResponse – структура для парсинга ответа DexScreener (упрощённо).
type DexScreenerResponse struct {
	Pairs []struct {
		DexID   string `json:"dexId"`
		Chain   string `json:"chain"`
		Address string `json:"address"`
	} `json:"pairs"`
}

// FindRaydiumPoolForNewToken ищет пул двумя способами:
// 1) Сначала через DexScreener API,
// 2) Если не найден – делает on‑chain сканирование аккаунтов Raydium AMM.
func FindRaydiumPoolForNewToken(ctx context.Context, tokenMint string, logger *zap.Logger) (string, error) {
	logger.Info("Searching Raydium pool for new token", zap.String("tokenMint", tokenMint))

	// Сначала пробуем через DexScreener.
	poolAddr, err := findViaDexScreener(ctx, tokenMint, logger)
	if err != nil {
		logger.Warn("DexScreener lookup failed; fallback to on-chain scanning", zap.Error(err))
	} else if poolAddr != "" {
		return poolAddr, nil
	}

	// Если через DexScreener ничего не найдено – используем on‑chain сканирование.
	logger.Info("Falling back to on-chain scanning for token", zap.String("tokenMint", tokenMint))
	return fallbackOnchainScan(ctx, tokenMint, logger)
}

// findViaDexScreener делает HTTP GET к DexScreener и пытается найти пул Raydium.
func findViaDexScreener(ctx context.Context, tokenMint string, logger *zap.Logger) (string, error) {
	apiURL := "https://api.dexscreener.com/latest/dex/tokens/" + tokenMint

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request for DexScreener: %w", err)
	}
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("DexScreener request error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("DexScreener returned status %d", resp.StatusCode)
	}

	var dexResp DexScreenerResponse
	if err := json.NewDecoder(resp.Body).Decode(&dexResp); err != nil {
		return "", fmt.Errorf("failed to decode DexScreener JSON: %w", err)
	}

	// Фильтруем пары: chain=solana, dexId=raydium.
	for _, pair := range dexResp.Pairs {
		if strings.EqualFold(pair.Chain, "solana") && strings.EqualFold(pair.DexID, "raydium") {
			if pair.Address != "" {
				logger.Info("Found Raydium pool from DexScreener", zap.String("poolAddress", pair.Address))
				return pair.Address, nil
			}
		}
	}
	// Если ничего не найдено – возвращаем пустую строку.
	return "", nil
}

// fallbackOnchainScan создает RPC-клиент, используя RPC-эндпоинт из конфигурационного файла,
// и вызывает on-chain сканирование.
func fallbackOnchainScan(ctx context.Context, tokenMint string, logger *zap.Logger) (string, error) {
	// Определяем локальную структуру для чтения конфигурации.
	type Config struct {
		RPCList []string `json:"rpc_list"`
	}

	// Значение по умолчанию.
	rpcEndpoint := "https://api.mainnet-beta.solana.com"

	// Пытаемся считать конфигурацию из файла.
	// Импорт "os" и "io/ioutil" (или os.ReadFile для Go 1.16+) должен быть добавлен в список импортов.
	data, err := os.ReadFile("configs/config.json")
	if err != nil {
		logger.Warn("Failed to read config file, using default RPC endpoint", zap.Error(err))
	} else {
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			logger.Warn("Failed to unmarshal config file, using default RPC endpoint", zap.Error(err))
		} else if len(cfg.RPCList) > 0 {
			rpcEndpoint = cfg.RPCList[0]
		}
	}

	// Создаем RPC-клиент с полученным эндпоинтом.
	c := rpc.New(rpcEndpoint)

	pools, err := FindPoolsForToken(ctx, tokenMint, c, logger)
	if err != nil {
		return "", fmt.Errorf("no pool found for token %s on on-chain scan: %w", tokenMint, err)
	}
	// Возвращаем адрес первого найденного пула.
	return pools[0].PoolAddress.String(), nil
}

// FindPoolsForToken выполняет on‑chain сканирование аккаунтов Raydium и возвращает список пулов,
// в которых участвует указанный токен.
func FindPoolsForToken(ctx context.Context, tokenMint string, client *rpc.Client, logger *zap.Logger) ([]PoolData, error) {
	tokenPubKey, err := solana.PublicKeyFromBase58(tokenMint)
	if err != nil {
		return nil, fmt.Errorf("invalid token mint address: %w", err)
	}

	// Используем фильтр по размеру аккаунта, чтобы выбрать пул-аккаунты.
	opts := rpc.GetProgramAccountsOpts{
		Filters: []rpc.RPCFilter{
			{DataSize: PoolAccountSize},
		},
		Commitment: rpc.CommitmentFinalized,
	}

	// Исправлено: вызываем метод GetProgramAccountsWithOpts (принимает 3 аргумента).
	accounts, err := client.GetProgramAccountsWithOpts(ctx, RaydiumV4ProgramID, &opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get program accounts for Raydium: %w", err)
	}

	var results []PoolData
	for _, acc := range accounts {
		data := acc.Account.Data.GetBinary()
		if len(data) < PoolAccountSize {
			continue
		}
		pool, err := parsePoolAccount(data, acc.Pubkey)
		if err != nil {
			logger.Debug("Failed to parse pool account", zap.Error(err))
			continue
		}
		if pool.BaseMint.Equals(tokenPubKey) || pool.QuoteMint.Equals(tokenPubKey) {
			logger.Info("Found Raydium pool for token", zap.String("poolAddress", acc.Pubkey.String()))
			results = append(results, pool)
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no pools found for token %s", tokenMint)
	}
	return results, nil
}

// parsePoolAccount декодирует бинарные данные пул-аккаунта и возвращает PoolData.
func parsePoolAccount(data []byte, poolAddress solana.PublicKey) (PoolData, error) {
	if len(data) < QuoteMintOffset+32 {
		return PoolData{}, fmt.Errorf("invalid pool account data length: %d", len(data))
	}

	var baseMintBytes, quoteMintBytes [32]byte
	copy(baseMintBytes[:], data[BaseMintOffset:BaseMintOffset+32])
	copy(quoteMintBytes[:], data[QuoteMintOffset:QuoteMintOffset+32])

	return PoolData{
		PoolAddress: poolAddress,
		BaseMint:    solana.PublicKey(baseMintBytes),
		QuoteMint:   solana.PublicKey(quoteMintBytes),
	}, nil
}

// PeriodicPoolScanner запускает периодическое сканирование пулов Raydium для заданного токена с указанным интервалом.
func PeriodicPoolScanner(ctx context.Context, tokenMint string, client *rpc.Client, logger *zap.Logger, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Info("Starting periodic pool scanner", zap.Duration("interval", interval))
	for {
		select {
		case <-ctx.Done():
			logger.Info("Stopping periodic pool scanner")
			return
		case <-ticker.C:
			pools, err := FindPoolsForToken(ctx, tokenMint, client, logger)
			if err != nil {
				logger.Error("Pool scanning error", zap.Error(err))
			} else {
				logger.Info("Pools found", zap.Any("pools", pools))
			}
		}
	}
}
