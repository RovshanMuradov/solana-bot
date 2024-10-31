// internal/dex/raydium/utils.go
package raydium

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/gagliardetto/solana-go"
	associatedtokenaccount "github.com/gagliardetto/solana-go/programs/associated-token-account"
	token "github.com/gagliardetto/solana-go/programs/token"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/rovshanmuradov/solana-bot/internal/blockchain"
)

// TokenAmountConfig содержит конфигурацию для работы с суммами токенов
type TokenAmountConfig struct {
	Amount   uint64
	Decimals uint8
}

// PriceImpactConfig содержит параметры для расчета влияния на цену
type PriceImpactConfig struct {
	AmountIn    uint64
	ReserveIn   uint64
	ReserveOut  uint64
	DecimalsIn  uint8
	DecimalsOut uint8
}

// RetryConfig содержит параметры для повторных попыток
type RetryConfig struct {
	MaxAttempts int
	Delay       time.Duration
	Factor      float64
}

// DefaultRetryConfig возвращает конфигурацию по умолчанию
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		Delay:       500 * time.Millisecond,
		Factor:      1.5,
	}
}

// TokenAmountToDecimal конвертирует uint64 в decimal с учетом decimals
func TokenAmountToDecimal(amount uint64, decimals uint8) decimal.Decimal {
	multiplier := decimal.New(1, int32(decimals))
	return decimal.NewFromInt(int64(amount)).Div(multiplier)
}

// DecimalToTokenAmount конвертирует decimal в uint64
func DecimalToTokenAmount(amount decimal.Decimal, decimals uint8) uint64 {
	multiplier := decimal.New(1, int32(decimals))
	result := amount.Mul(multiplier)
	if !result.IsInteger() {
		return 0
	}
	return uint64(result.IntPart())
}

// CalculatePriceImpact вычисляет влияние сделки на цену
func CalculatePriceImpact(cfg PriceImpactConfig) (decimal.Decimal, error) {
	if cfg.ReserveIn == 0 || cfg.ReserveOut == 0 {
		return decimal.Zero, fmt.Errorf("reserves cannot be zero")
	}

	amountIn := TokenAmountToDecimal(cfg.AmountIn, cfg.DecimalsIn)
	reserveIn := TokenAmountToDecimal(cfg.ReserveIn, cfg.DecimalsIn)
	reserveOut := TokenAmountToDecimal(cfg.ReserveOut, cfg.DecimalsOut)

	// Расчет влияния на цену: (new_price - old_price) / old_price * 100
	oldPrice := reserveOut.Div(reserveIn)
	newReserveIn := reserveIn.Add(amountIn)
	newPrice := reserveOut.Div(newReserveIn)

	impact := oldPrice.Sub(newPrice).Div(oldPrice).Mul(decimal.NewFromInt(100))
	return impact.Abs(), nil
}

// FormatTokenAmount форматирует сумму токена с учетом decimals
func FormatTokenAmount(cfg TokenAmountConfig) string {
	amount := TokenAmountToDecimal(cfg.Amount, cfg.Decimals)
	return amount.StringFixed(int32(cfg.Decimals))
}

// WithRetry выполняет функцию с повторными попытками
func WithRetry[T any](
	ctx context.Context,
	operation func(context.Context) (T, error),
	cfg RetryConfig,
	logger *zap.Logger,
) (T, error) {
	var result T
	var lastErr error

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
			if attempt > 0 {
				delay := time.Duration(float64(cfg.Delay) * math.Pow(cfg.Factor, float64(attempt-1)))
				logger.Debug("Retrying operation",
					zap.Int("attempt", attempt+1),
					zap.Duration("delay", delay))
				time.Sleep(delay)
			}

			result, lastErr = operation(ctx)
			if lastErr == nil {
				return result, nil
			}

			logger.Warn("Operation failed",
				zap.Int("attempt", attempt+1),
				zap.Error(lastErr))
		}
	}

	return result, fmt.Errorf("operation failed after %d attempts: %w", cfg.MaxAttempts, lastErr)
}

// FindAssociatedTokenAddress находит или создает ATA
func FindAssociatedTokenAddress(
	ctx context.Context,
	client blockchain.Client,
	owner, mint solana.PublicKey,
	logger *zap.Logger,
) (solana.PublicKey, []solana.Instruction, error) {
	ata, _, err := solana.FindAssociatedTokenAddress(owner, mint)
	if err != nil {
		return solana.PublicKey{}, nil, fmt.Errorf("failed to find ATA: %w", err)
	}

	// Проверяем существование аккаунта
	account, err := client.GetAccountInfo(ctx, ata)
	if err != nil {
		return solana.PublicKey{}, nil, fmt.Errorf("failed to get account info: %w", err)
	}

	var instructions []solana.Instruction
	if account == nil || account.Value == nil {
		// Создаем инструкцию для создания ATA
		createATAInst := associatedtokenaccount.NewCreateInstruction(
			owner, // payer
			ata,   // ata
			owner, // owner
		).Build()

		instructions = append(instructions, createATAInst)

		logger.Debug("ATA creation instruction added",
			zap.String("ata", ata.String()),
			zap.String("owner", owner.String()),
			zap.String("mint", mint.String()))
	}

	return ata, instructions, nil
}

// CalculateMinimumReceived вычисляет минимальное количество токенов с учетом проскальзывания
func CalculateMinimumReceived(amount uint64, slippageBps uint16) uint64 {
	if slippageBps >= 10000 {
		return 0
	}

	slippage := decimal.NewFromInt(int64(slippageBps)).Div(decimal.NewFromInt(10000))
	amountDec := decimal.NewFromInt(int64(amount))
	minimum := amountDec.Mul(decimal.NewFromInt(1).Sub(slippage))

	return uint64(minimum.IntPart())
}

// DecodePDA декодирует Program Derived Address
func DecodePDA(seeds [][]byte, programID solana.PublicKey) (solana.PublicKey, uint8, error) {
	if len(seeds) == 0 {
		return solana.PublicKey{}, 0, fmt.Errorf("empty seeds")
	}

	for nonce := uint8(0); nonce < 255; nonce++ {
		nonceBytes := []byte{nonce}
		allSeeds := append(append([][]byte{}, seeds...), nonceBytes)

		addr, err := solana.CreateProgramAddress(allSeeds, programID)
		if err == nil {
			return addr, nonce, nil
		}
	}

	return solana.PublicKey{}, 0, fmt.Errorf("could not find valid PDA")
}

// ValidateAndAdjustSlippage проверяет и корректирует значение проскальзывания
func ValidateAndAdjustSlippage(slippageBps uint16) uint16 {
	if slippageBps == 0 {
		return 50 // 0.5% по умолчанию
	}
	if slippageBps > 10000 { // Максимум 100%
		return 10000
	}
	return slippageBps
}

// EstimateRequiredSOL оценивает необходимое количество SOL для транзакции
func EstimateRequiredSOL(
	instructions []solana.Instruction,
	recentBlockhash solana.Hash,
	priorityFee uint64,
) uint64 {
	// Базовая стоимость транзакции
	const baseTransactionCost = 5000

	// Стоимость подписи (за каждого подписанта)
	const signatureCost = 1000

	// Оценка стоимости инструкций (приблизительно)
	instructionCost := uint64(len(instructions)) * 1000

	// Учитываем приоритетную комиссию
	totalCost := baseTransactionCost + signatureCost + instructionCost + priorityFee

	// Добавляем запас 10%
	return uint64(float64(totalCost) * 1.1)
}

// ParsePoolAddress парсит и проверяет адрес пула
func ParsePoolAddress(address string) (solana.PublicKey, error) {
	if address == "" {
		return solana.PublicKey{}, fmt.Errorf("empty pool address")
	}

	poolPubkey, err := solana.PublicKeyFromBase58(address)
	if err != nil {
		return solana.PublicKey{}, fmt.Errorf("invalid pool address: %w", err)
	}

	if poolPubkey.IsZero() {
		return solana.PublicKey{}, fmt.Errorf("zero pool address")
	}

	return poolPubkey, nil
}

// ValidateTokenAccount проверяет токен-аккаунт
func ValidateTokenAccount(
	ctx context.Context,
	client blockchain.Client,
	account solana.PublicKey,
	expectedMint solana.PublicKey,
	expectedOwner solana.PublicKey,
) error {
	info, err := client.GetAccountInfo(ctx, account)
	if err != nil {
		return fmt.Errorf("failed to get account info: %w", err)
	}

	if info.Value == nil {
		return fmt.Errorf("account not found")
	}

	if !info.Value.Owner.Equals(token.ProgramID) {
		return fmt.Errorf("invalid account owner")
	}

	var tokenAccount token.Account
	if err := binary.Read(bytes.NewReader(info.Value.Data.GetBinary()), binary.LittleEndian, &tokenAccount); err != nil {
		return fmt.Errorf("failed to decode token account: %w", err)
	}

	if !tokenAccount.Mint.Equals(expectedMint) {
		return fmt.Errorf("invalid mint")
	}

	if !tokenAccount.Owner.Equals(expectedOwner) {
		return fmt.Errorf("invalid owner")
	}

	return nil
}

// ConvertBigFloatToUint64 безопасно конвертирует big.Float в uint64
func ConvertBigFloatToUint64(value *big.Float) (uint64, error) {
	if value == nil {
		return 0, fmt.Errorf("nil value")
	}

	if value.Sign() < 0 {
		return 0, fmt.Errorf("negative value")
	}

	// Проверяем, что значение не превышает максимальное значение uint64
	maxUint64 := new(big.Float).SetUint64(math.MaxUint64)
	if value.Cmp(maxUint64) > 0 {
		return 0, fmt.Errorf("value exceeds uint64 range")
	}

	result, accuracy := value.Uint64()
	if accuracy != big.Exact {
		return 0, fmt.Errorf("value has fractional part or exceeds uint64")
	}

	return result, nil
}

// Утилиты для версионированных транзакций
func CreateVersionedTransaction(instructions []solana.Instruction, lookupTables []solana.AddressLookupTable) (*solana.VersionedTransaction, error) {
	// Создаем транзакцию
	return nil, nil
}

// Утилиты для работы с новыми комиссиями
func CalculateV5Fees(amount uint64, feeParams V5FeeParams) uint64 {
	return 0
}
