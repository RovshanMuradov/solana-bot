package pumpswap_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/rovshanmuradov/solana-bot/internal/dex/model"
	"github.com/stretchr/testify/assert"
)

// PriceUpdate представляет обновление цены токена (копия из monitor.PriceUpdate)
type PriceUpdate struct {
	Current float64 // Текущая цена токена
	Initial float64 // Начальная цена токена
	Percent float64 // Процентное изменение цены
	Tokens  float64 // Количество токенов
}

// DEXMock реализует интерфейс DEX с предопределенными значениями для тестирования
type DEXMock struct {
	Price        float64
	Balance      float64
	SellEstimate float64
	Initial      float64
	TokenMint    string
	TokenName    string
}

// GetName возвращает название биржи.
func (m *DEXMock) GetName() string {
	return "PumpSwap Mock"
}

// Execute выполняет операцию, описанную в задаче.
func (m *DEXMock) Execute(ctx context.Context, task interface{}) error {
	return nil
}

// GetTokenPrice возвращает текущую цену токена
func (m *DEXMock) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	return m.Price, nil
}

// GetTokenBalance возвращает текущий баланс токена в кошельке пользователя
func (m *DEXMock) GetTokenBalance(ctx context.Context, tokenMint string) (uint64, error) {
	// Предполагаем, что баланс в токенах хранится с некоторым множителем (например, 10^6)
	return uint64(m.Balance * 1_000_000), nil
}

// SellPercentTokens продает указанный процент имеющихся токенов
func (m *DEXMock) SellPercentTokens(ctx context.Context, tokenMint string, percentToSell float64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
	return nil
}

// CalculatePnL вычисляет метрики прибыли и убытка для заданного количества токенов и начальных инвестиций
func (m *DEXMock) CalculatePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*model.PnLResult, error) {
	// Возвращаем предопределенные значения для тестирования
	investment := initialInvestment
	if investment == 0 {
		investment = m.Initial * m.Balance // если не задано, вычисляем как начальная_цена * количество_токенов
	}

	return &model.PnLResult{
		InitialInvestment: investment,
		SellEstimate:      m.SellEstimate,
		NetPnL:            m.SellEstimate - investment,
		PnLPercentage:     ((m.SellEstimate - investment) / investment) * 100,
	}, nil
}

// RenderUIMonitor создает строку, форматированную как UI для мониторинга токена
func RenderUIMonitor(dex *DEXMock) string {
	// Создаем PriceUpdate из данных в DEXMock
	update := PriceUpdate{
		Current: dex.Price,
		Initial: dex.Initial,
		Percent: ((dex.Price - dex.Initial) / dex.Initial) * 100,
		Tokens:  dex.Balance,
	}

	// Создаем PnLResult
	pnl := model.PnLResult{
		InitialInvestment: dex.Initial * dex.Balance,
		SellEstimate:      dex.SellEstimate,
		NetPnL:            dex.SellEstimate - (dex.Initial * dex.Balance),
		PnLPercentage:     ((dex.SellEstimate - (dex.Initial * dex.Balance)) / (dex.Initial * dex.Balance)) * 100,
	}

	// Получаем токен мин или используем дефолтный
	tokenMint := dex.TokenMint
	if tokenMint == "" {
		tokenMint = "FAKE..."
	}

	// Реализуем логику форматирования UI
	var sb strings.Builder

	// Заголовок
	sb.WriteString("╔════════════════ TOKEN MONITOR ════════════════╗\n")
	sb.WriteString(fmt.Sprintf("║ Token: %-38s ║\n", tokenMint))
	sb.WriteString("╟───────────────────────────────────────────────╢\n")

	// Информация о цене
	sb.WriteString(fmt.Sprintf("║ Current Price:       %.8f SOL           ║\n", update.Current))
	sb.WriteString(fmt.Sprintf("║ Initial Price:       %.8f SOL           ║\n", update.Initial))

	// Процентное изменение (без цветов для тестирования)
	changeStr := fmt.Sprintf("%.2f%%", update.Percent)
	sb.WriteString(fmt.Sprintf("║ Price Change:        %-24s ║\n", changeStr))

	// Количество токенов
	sb.WriteString(fmt.Sprintf("║ Tokens Owned:        %-14.8f           ║\n", update.Tokens))
	sb.WriteString("╟───────────────────────────────────────────────╢\n")

	// Информация о PnL
	sb.WriteString(fmt.Sprintf("║ Sold (Estimate):     %.8f SOL           ║\n", pnl.SellEstimate))
	sb.WriteString(fmt.Sprintf("║ Invested:            %.8f SOL           ║\n", pnl.InitialInvestment))

	// PnL (без цветов для тестирования)
	pnlStr := fmt.Sprintf("−%.8f SOL (−%.2f%%)", -pnl.NetPnL, -pnl.PnLPercentage)
	if pnl.NetPnL >= 0 {
		pnlStr = fmt.Sprintf("%.8f SOL (%.2f%%)", pnl.NetPnL, pnl.PnLPercentage)
	}
	sb.WriteString(fmt.Sprintf("║ P&L:                 %-24s ║\n", pnlStr))
	sb.WriteString("╚═══════════════════════════════════════════════╝")

	return sb.String()
}

// TestFullUITemplate тестирует функцию отображения UI для мониторинга
func TestFullUITemplate(t *testing.T) {
	fake := &DEXMock{
		Price:        0.50000000,
		Balance:      2.00000000,
		SellEstimate: 0.99500000,
		Initial:      0.50000000,
	}

	out := RenderUIMonitor(fake)
	want :=
		`╔════════════════ TOKEN MONITOR ════════════════╗
║ Token: FAKE...                                ║
╟───────────────────────────────────────────────╢
║ Current Price:       0.50000000 SOL           ║
║ Initial Price:       0.50000000 SOL           ║
║ Price Change:        0.00%                    ║
║ Tokens Owned:        2.00000000               ║
╟───────────────────────────────────────────────╢
║ Sold (Estimate):     0.99500000 SOL           ║
║ Invested:            1.00000000 SOL           ║
║ P&L:                 −0.00500000 SOL (−0.50%) ║
╚═══════════════════════════════════════════════╝`

	assert.Equal(t, want, out)
}
