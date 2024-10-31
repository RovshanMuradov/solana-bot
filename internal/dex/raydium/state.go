// internal/dex/raydium/stage.go
package raydium

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

// StateVersion определяет версию структуры состояния
type StateVersion uint8

const (
	StateV4 StateVersion = 4
	StateV5 StateVersion = 5
)

// StateStatus определяет статус пула
type StateStatus uint8

const (
	StateUninitialized StateStatus = iota
	StateInitialized
	StateFrozen
	StateWithdrawn
)

// Layout определяет структуру данных состояния пула
type Layout struct {
	// Базовая информация
	Discriminator [8]byte // Уникальный идентификатор программы
	Version       uint8   // Версия структуры данных
	Status        uint8   // Статус пула
	Nonce         uint8   // Nonce для генерации PDA

	// Минты и декодеры
	BaseMint      solana.PublicKey // Базовый токен
	QuoteMint     solana.PublicKey // Котируемый токен
	LPMint        solana.PublicKey // LP токен
	BaseDecimals  uint8            // Decimal базового токена
	QuoteDecimals uint8            // Decimal котируемого токена
	LPDecimals    uint8            // Decimal LP токена

	// Параметры пула
	BaseReserve        uint64 // Резерв базового токена
	QuoteReserve       uint64 // Резерв котируемого токена
	LPSupply           uint64 // Общее количество LP токенов
	SwapFeeNumerator   uint64 // Числитель комиссии
	SwapFeeDenominator uint64 // Знаменатель комиссии

	// Аккаунты
	BaseVault     solana.PublicKey // Vault для базового токена
	QuoteVault    solana.PublicKey // Vault для котируемого токена
	WithdrawQueue solana.PublicKey // Очередь вывода средств
	LPVault       solana.PublicKey // Vault для LP токенов
	Owner         solana.PublicKey // Владелец пула

	// Serum параметры
	MarketId         solana.PublicKey // ID рынка Serum
	OpenOrders       solana.PublicKey // Открытые ордера
	TargetOrders     solana.PublicKey // Целевые ордера
	MarketBaseVault  solana.PublicKey // Базовый vault рынка
	MarketQuoteVault solana.PublicKey // Котируемый vault рынка
	MarketBids       solana.PublicKey // Биды рынка
	MarketAsks       solana.PublicKey // Аски рынка
}

// StateDecoder отвечает за декодирование состояния пула
type StateDecoder struct {
	logger *zap.Logger
}

// NewStateDecoder создает новый декодер состояния
func NewStateDecoder(logger *zap.Logger) *StateDecoder {
	return &StateDecoder{
		logger: logger.Named("state-decoder"),
	}
}

// DecodeState декодирует бинарные данные в структуру состояния
func (d *StateDecoder) DecodeState(data []byte) (*Layout, error) {
	if len(data) < LayoutBaseSize {
		return nil, fmt.Errorf("insufficient data length: got %d, need at least %d", len(data), LayoutBaseSize)
	}

	logger := d.logger.With(zap.Int("data_length", len(data)))
	logger.Debug("Starting state decoding")

	layout := &Layout{}

	// Читаем базовую информацию
	copy(layout.Discriminator[:], data[:8])
	layout.Version = data[8]
	layout.Status = data[9]
	layout.Nonce = data[10]

	// Проверяем версию
	if layout.Version != uint8(StateV4) && layout.Version != uint8(StateV5) {
		return nil, fmt.Errorf("unsupported state version: %d", layout.Version)
	}

	// Декодируем публичные ключи и другие данные
	offset := LayoutBaseSize

	// Функция-помощник для чтения PublicKey
	readPubKey := func() (solana.PublicKey, error) {
		if offset+32 > len(data) {
			return solana.PublicKey{}, fmt.Errorf("insufficient data for public key at offset %d", offset)
		}
		var key solana.PublicKey
		copy(key[:], data[offset:offset+32])
		offset += 32
		return key, nil
	}

	// Функция-помощник для чтения uint64
	readUint64 := func() (uint64, error) {
		if offset+8 > len(data) {
			return 0, fmt.Errorf("insufficient data for uint64 at offset %d", offset)
		}
		value := binary.LittleEndian.Uint64(data[offset : offset+8])
		offset += 8
		return value, nil
	}

	// Функция-помощник для чтения uint8
	readUint8 := func() (uint8, error) {
		if offset+1 > len(data) {
			return 0, fmt.Errorf("insufficient data for uint8 at offset %d", offset)
		}
		value := data[offset]
		offset += 1
		return value, nil
	}

	var err error

	// Читаем минты
	if layout.BaseMint, err = readPubKey(); err != nil {
		return nil, fmt.Errorf("failed to read base mint: %w", err)
	}
	if layout.QuoteMint, err = readPubKey(); err != nil {
		return nil, fmt.Errorf("failed to read quote mint: %w", err)
	}
	if layout.LPMint, err = readPubKey(); err != nil {
		return nil, fmt.Errorf("failed to read LP mint: %w", err)
	}

	// Читаем decimals
	if layout.BaseDecimals, err = readUint8(); err != nil {
		return nil, fmt.Errorf("failed to read base decimals: %w", err)
	}
	if layout.QuoteDecimals, err = readUint8(); err != nil {
		return nil, fmt.Errorf("failed to read quote decimals: %w", err)
	}
	if layout.LPDecimals, err = readUint8(); err != nil {
		return nil, fmt.Errorf("failed to read LP decimals: %w", err)
	}

	// Читаем резервы и параметры пула
	if layout.BaseReserve, err = readUint64(); err != nil {
		return nil, fmt.Errorf("failed to read base reserve: %w", err)
	}
	if layout.QuoteReserve, err = readUint64(); err != nil {
		return nil, fmt.Errorf("failed to read quote reserve: %w", err)
	}
	if layout.LPSupply, err = readUint64(); err != nil {
		return nil, fmt.Errorf("failed to read LP supply: %w", err)
	}
	if layout.SwapFeeNumerator, err = readUint64(); err != nil {
		return nil, fmt.Errorf("failed to read swap fee numerator: %w", err)
	}
	if layout.SwapFeeDenominator, err = readUint64(); err != nil {
		return nil, fmt.Errorf("failed to read swap fee denominator: %w", err)
	}

	// Читаем аккаунты
	if layout.BaseVault, err = readPubKey(); err != nil {
		return nil, fmt.Errorf("failed to read base vault: %w", err)
	}
	if layout.QuoteVault, err = readPubKey(); err != nil {
		return nil, fmt.Errorf("failed to read quote vault: %w", err)
	}
	if layout.WithdrawQueue, err = readPubKey(); err != nil {
		return nil, fmt.Errorf("failed to read withdraw queue: %w", err)
	}
	if layout.LPVault, err = readPubKey(); err != nil {
		return nil, fmt.Errorf("failed to read LP vault: %w", err)
	}
	if layout.Owner, err = readPubKey(); err != nil {
		return nil, fmt.Errorf("failed to read owner: %w", err)
	}

	// Читаем Serum параметры
	if layout.MarketId, err = readPubKey(); err != nil {
		return nil, fmt.Errorf("failed to read market ID: %w", err)
	}
	if layout.OpenOrders, err = readPubKey(); err != nil {
		return nil, fmt.Errorf("failed to read open orders: %w", err)
	}
	if layout.TargetOrders, err = readPubKey(); err != nil {
		return nil, fmt.Errorf("failed to read target orders: %w", err)
	}
	if layout.MarketBaseVault, err = readPubKey(); err != nil {
		return nil, fmt.Errorf("failed to read market base vault: %w", err)
	}
	if layout.MarketQuoteVault, err = readPubKey(); err != nil {
		return nil, fmt.Errorf("failed to read market quote vault: %w", err)
	}
	if layout.MarketBids, err = readPubKey(); err != nil {
		return nil, fmt.Errorf("failed to read market bids: %w", err)
	}
	if layout.MarketAsks, err = readPubKey(); err != nil {
		return nil, fmt.Errorf("failed to read market asks: %w", err)
	}

	logger.Debug("State decoded successfully",
		zap.Uint8("version", layout.Version),
		zap.Uint8("status", layout.Status),
		zap.Uint64("base_reserve", layout.BaseReserve),
		zap.Uint64("quote_reserve", layout.QuoteReserve))

	return layout, nil
}

// GetStateSize возвращает ожидаемый размер состояния для версии
func (d *StateDecoder) GetStateSize(version StateVersion) uint64 {
	switch version {
	case StateV4:
		return 752
	case StateV5:
		return 824
	default:
		return 0
	}
}

// ValidateState проверяет корректность декодированного состояния
func (d *StateDecoder) ValidateState(layout *Layout) error {
	if layout == nil {
		return fmt.Errorf("layout is nil")
	}

	// Проверяем базовые параметры
	if layout.Status > uint8(StateWithdrawn) {
		return fmt.Errorf("invalid state status: %d", layout.Status)
	}

	if layout.BaseDecimals > 32 || layout.QuoteDecimals > 32 || layout.LPDecimals > 32 {
		return fmt.Errorf("invalid decimals")
	}

	// Проверяем комиссию
	if layout.SwapFeeDenominator == 0 {
		return fmt.Errorf("swap fee denominator cannot be zero")
	}

	feeRate := float64(layout.SwapFeeNumerator) / float64(layout.SwapFeeDenominator)
	if feeRate > 0.1 { // Максимальная комиссия 10%
		return fmt.Errorf("swap fee too high: %f", feeRate)
	}

	// Проверяем публичные ключи
	if layout.BaseMint.IsZero() || layout.QuoteMint.IsZero() || layout.LPMint.IsZero() {
		return fmt.Errorf("invalid mint addresses")
	}

	// Проверяем резервы
	if layout.Status == uint8(StateInitialized) {
		if layout.BaseReserve == 0 || layout.QuoteReserve == 0 {
			return fmt.Errorf("reserves cannot be zero for initialized pool")
		}
	}

	return nil
}

// CalculateVirtualPrice вычисляет виртуальную цену LP токена
func (d *StateDecoder) CalculateVirtualPrice(layout *Layout) (float64, error) {
	if layout.LPSupply == 0 {
		return 0, fmt.Errorf("LP supply is zero")
	}

	baseAdjusted := float64(layout.BaseReserve) / math.Pow10(int(layout.BaseDecimals))
	quoteAdjusted := float64(layout.QuoteReserve) / math.Pow10(int(layout.QuoteDecimals))
	lpAdjusted := float64(layout.LPSupply) / math.Pow10(int(layout.LPDecimals))

	// Вычисляем геометрическое среднее резервов
	sqrtK := math.Sqrt(baseAdjusted * quoteAdjusted)
	return 2 * sqrtK / lpAdjusted, nil
}

// Декодер для v5 состояния
func (d *StateDecoder) DecodeV5State(data []byte) (*LayoutV5, error) {
	// ... реализация декодирования v5 состояния
	// Этот метод будет реализован в следующей части
	return nil, nil
}

// Методы миграции
func (d *StateDecoder) MigrateState(oldState *Layout, newVersion StateVersion) (*Layout, error) {
	// ... реализация миграции состояния
	// Этот метод будет реализован в следующей части
	return nil, nil
}
