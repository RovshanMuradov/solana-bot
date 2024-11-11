// internal/dex/raydium/client.go
package raydium

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/token"
	solanarpc "github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"go.uber.org/zap"
)

// NewRaydiumClient создает новый экземпляр клиента
func NewRaydiumClient(rpcEndpoint string, wallet solana.PrivateKey, logger *zap.Logger) (*Client, error) {
	logger = logger.Named("raydium-client")

	solClient, err := solbc.NewClient(
		[]string{rpcEndpoint},
		wallet,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create solana client: %w", err)
	}

	return &Client{
		client:      solClient,
		logger:      logger,
		privateKey:  wallet,
		timeout:     30 * time.Second,
		retries:     3,
		priorityFee: 1000,
		commitment:  solanarpc.CommitmentConfirmed,
		poolCache:   NewPoolCache(logger),
		api:         NewAPIService(logger),
	}, nil
}

// GetPool получает информацию о пуле с поддержкой поиска в обоих направлениях
func (c *Client) GetPool(ctx context.Context, baseMint, quoteMint solana.PublicKey) (*Pool, error) {
	c.logger.Debug("searching pool",
		zap.String("baseMint", baseMint.String()),
		zap.String("quoteMint", quoteMint.String()))

	// Проверяем кэш в обоих направлениях
	if pool := c.poolCache.GetPool(baseMint, quoteMint); pool != nil {
		// Проверяем ликвидность закэшированного пула
		if err := c.checkPoolLiquidity(ctx, pool); err == nil {
			return pool, nil
		}
		c.logger.Debug("cached pool has insufficient liquidity, searching new pool")
	}

	// Пробуем найти пул через API в обоих направлениях
	pool, err := c.api.GetPoolByToken(ctx, baseMint)
	if err == nil {
		if err := c.enrichAndValidatePool(ctx, pool, baseMint, quoteMint); err == nil {
			return c.cacheAndReturn(pool), nil
		}
	}

	pool, err = c.api.GetPoolByToken(ctx, quoteMint)
	if err == nil {
		if err := c.enrichAndValidatePool(ctx, pool, baseMint, quoteMint); err == nil {
			return c.cacheAndReturn(pool), nil
		}
	}

	return nil, fmt.Errorf("pool not found for tokens %s and %s", baseMint, quoteMint)
}

// enrichAndValidatePool обогащает пул данными и проверяет его валидность
func (c *Client) enrichAndValidatePool(ctx context.Context, pool *Pool, baseMint, quoteMint solana.PublicKey) error {
	// Получаем данные пула из блокчейна
	account, err := c.client.GetAccountInfo(ctx, pool.ID)
	if err != nil {
		return fmt.Errorf("failed to get pool account: %w", err)
	}

	if account == nil || account.Value == nil || len(account.Value.Data.GetBinary()) < PoolAccountSize {
		return fmt.Errorf("invalid pool account data")
	}

	data := account.Value.Data.GetBinary()

	// Получаем authority через PDA
	authority, _, err := solana.FindProgramAddress(
		[][]byte{[]byte(AmmAuthorityLayout)},
		RaydiumV4ProgramID,
	)
	if err != nil {
		return fmt.Errorf("failed to derive authority: %w", err)
	}

	// Обновляем данные пула
	pool.Authority = authority
	pool.BaseMint = solana.PublicKeyFromBytes(data[BaseMintOffset : BaseMintOffset+32])
	pool.QuoteMint = solana.PublicKeyFromBytes(data[QuoteMintOffset : QuoteMintOffset+32])
	pool.BaseVault = solana.PublicKeyFromBytes(data[BaseVaultOffset : BaseVaultOffset+32])
	pool.QuoteVault = solana.PublicKeyFromBytes(data[QuoteVaultOffset : QuoteVaultOffset+32])
	pool.BaseDecimals = data[DecimalsOffset]
	pool.QuoteDecimals = data[DecimalsOffset+1]
	pool.Version = PoolVersionV4

	// Обновляем состояние
	pool.State = PoolState{
		BaseReserve:  binary.LittleEndian.Uint64(data[64:72]),
		QuoteReserve: binary.LittleEndian.Uint64(data[72:80]),
		Status:       data[88],
	}

	// Проверяем соответствие токенов
	if !((pool.BaseMint.Equals(baseMint) && pool.QuoteMint.Equals(quoteMint)) ||
		(pool.BaseMint.Equals(quoteMint) && pool.QuoteMint.Equals(baseMint))) {
		return fmt.Errorf("pool tokens do not match requested tokens")
	}

	return c.checkPoolLiquidity(ctx, pool)
}

// checkPoolLiquidity проверяет достаточность ликвидности в пуле
func (c *Client) checkPoolLiquidity(ctx context.Context, pool *Pool) error {
	if pool.State.Status != PoolStatusActive {
		return fmt.Errorf("pool is not active")
	}

	if pool.State.BaseReserve == 0 || pool.State.QuoteReserve == 0 {
		return fmt.Errorf("pool has no liquidity")
	}

	return nil
}

// ensureTokenAccounts проверяет и создает ATA при необходимости
func (c *Client) ensureTokenAccounts(ctx context.Context, sourceToken, targetToken solana.PublicKey) (*TokenAccounts, error) {
	accounts := &TokenAccounts{}
	var created bool

	// Получаем ATA для source токена
	sourceATA, _, err := solana.FindAssociatedTokenAddress(
		c.GetPublicKey(),
		sourceToken,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find source ATA: %w", err)
	}
	accounts.SourceATA = sourceATA

	// Получаем ATA для target токена
	targetATA, _, err := solana.FindAssociatedTokenAddress(
		c.GetPublicKey(),
		targetToken,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to find target ATA: %w", err)
	}
	accounts.DestinationATA = targetATA

	// Проверяем существование ATA
	if exists, err := c.checkTokenAccount(ctx, sourceATA); err != nil {
		return nil, err
	} else if !exists {
		if err := c.createTokenAccount(ctx, sourceToken); err != nil {
			return nil, err
		}
		created = true
	}

	if exists, err := c.checkTokenAccount(ctx, targetATA); err != nil {
		return nil, err
	} else if !exists {
		if err := c.createTokenAccount(ctx, targetToken); err != nil {
			return nil, err
		}
		created = true
	}

	accounts.Created = created
	return accounts, nil
}

// prepareSwap подготавливает параметры для свапа
func (c *Client) prepareSwap(ctx context.Context, params *SwapParams) error {
	// Проверяем балансы
	balance, err := c.client.GetBalance(ctx, params.UserWallet, c.commitment)
	if err != nil {
		return fmt.Errorf("failed to get wallet balance: %w", err)
	}

	requiredBalance := params.AmountIn + params.PriorityFeeLamports + 5000 // 5000 lamports для комиссии
	if balance < requiredBalance {
		return fmt.Errorf("insufficient balance: required %d, got %d", requiredBalance, balance)
	}

	// Проверяем и создаем ATA если необходимо
	accounts, err := c.ensureTokenAccounts(ctx, params.Pool.BaseMint, params.Pool.QuoteMint)
	if err != nil {
		return fmt.Errorf("failed to ensure token accounts: %w", err)
	}
	params.SourceTokenAccount = accounts.SourceATA
	params.DestinationTokenAccount = accounts.DestinationATA

	// Проверяем пул
	if err := c.checkPoolLiquidity(ctx, params.Pool); err != nil {
		return fmt.Errorf("pool liquidity check failed: %w", err)
	}

	return nil
}

// Вспомогательные методы

func (c *Client) checkTokenAccount(ctx context.Context, account solana.PublicKey) (bool, error) {
	acc, err := c.client.GetAccountInfo(ctx, account)
	if err != nil {
		return false, fmt.Errorf("failed to get token account: %w", err)
	}
	return acc != nil && acc.Value != nil, nil
}

func (c *Client) createTokenAccount(ctx context.Context, mint solana.PublicKey) error {
	// Создаем инструкцию для создания ассоциированного токен аккаунта
	instruction := token.NewInitializeAccount3InstructionBuilder().
		SetAccount(c.GetPublicKey()).
		SetMintAccount(mint).
		SetOwner(c.GetPublicKey()).
		Build()

	// Получаем последний блокхэш
	recentBlockHash, err := c.client.GetRecentBlockhash(ctx)
	if err != nil {
		return fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Создаем транзакцию
	tx, err := solana.NewTransaction(
		[]solana.Instruction{instruction},
		recentBlockHash,
	)
	if err != nil {
		return fmt.Errorf("failed to create transaction: %w", err)
	}

	// Отправляем транзакцию
	_, err = c.client.SendTransaction(ctx, tx)
	if err != nil {
		return fmt.Errorf("failed to create token account: %w", err)
	}

	return nil
}

func (c *Client) cacheAndReturn(pool *Pool) *Pool {
	if err := c.poolCache.AddPool(pool); err != nil {
		c.logger.Warn("failed to cache pool", zap.Error(err))
	}
	return pool
}

func (c *Client) GetPublicKey() solana.PublicKey {
	return c.privateKey.PublicKey()
}

func (c *Client) GetBaseClient() blockchain.Client {
	return c.client
}
