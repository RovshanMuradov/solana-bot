package pumpswap_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/rovshanmuradov/solana-bot/internal/dex/model"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// RealDEXMock реализует тестовую версию DEX с конкретными значениями расчетов
type RealDEXMock struct {
	price         float64
	balance       float64
	sellEstimate  float64
	initial       float64
	tokenMint     string
	initialInvest float64
	logger        *zap.Logger
}

// GetName возвращает название биржи
func (m *RealDEXMock) GetName() string {
	return "PumpSwap RealMock"
}

// Execute имитация выполнения операции
func (m *RealDEXMock) Execute(ctx context.Context, task interface{}) error {
	return nil
}

// GetTokenPrice возвращает текущую цену токена
func (m *RealDEXMock) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	return m.price, nil
}

// GetTokenBalance возвращает текущий баланс токена
func (m *RealDEXMock) GetTokenBalance(ctx context.Context, tokenMint string) (uint64, error) {
	// Предполагаем, что баланс в токенах хранится с 6 десятичными знаками (10^6)
	return uint64(m.balance * 1_000_000), nil
}

// SellPercentTokens имитация продажи процента токенов
func (m *RealDEXMock) SellPercentTokens(ctx context.Context, tokenMint string, percentToSell float64, slippagePercent float64, priorityFeeSol string, computeUnits uint32) error {
	return nil
}

// CalculatePnL вычисляет реальные метрики прибыли и убытка
func (m *RealDEXMock) CalculatePnL(ctx context.Context, tokenAmount float64, initialInvestment float64) (*model.PnLResult, error) {
	// Используем тот же код, который реализован в DEX.CalculatePnL
	dexFeePercent := 0.25 // Процент комиссии DEX

	// Из начальной инвестиции вычитаем комиссию при покупке
	buyFee := initialInvestment * (dexFeePercent / 100.0)
	costBasis := initialInvestment - buyFee

	// Используем предустановленное значение для sellEstimate
	sellEstimate := m.sellEstimate

	// Вычисляем чистую прибыль/убыток
	netPnL := sellEstimate - costBasis

	// Вычисляем процент прибыли/убытка
	pnlPercentage := 0.0
	if costBasis > 0 {
		pnlPercentage = (netPnL / costBasis) * 100
	}

	// Формируем результат
	result := &model.PnLResult{
		SellEstimate:      sellEstimate,
		InitialInvestment: costBasis,
		NetPnL:            netPnL,
		PnLPercentage:     pnlPercentage,
	}

	return result, nil
}

// PriceUpdate представляет обновление цены токена для теста
type UIMonitorPriceUpdate struct {
	Current float64 // Текущая цена токена
	Initial float64 // Начальная цена токена
	Percent float64 // Процентное изменение цены
	Tokens  float64 // Количество токенов
}

// RenderRealUIMonitor создает строку, форматированную как UI для мониторинга токена
func RenderRealUIMonitor(dex *RealDEXMock) string {
	// Создаем PriceUpdate
	update := UIMonitorPriceUpdate{
		Current: dex.price,
		Initial: dex.initial,
		Percent: ((dex.price - dex.initial) / dex.initial) * 100,
		Tokens:  dex.balance,
	}

	// Получаем PnLResult
	pnl, _ := dex.CalculatePnL(context.Background(), dex.balance, dex.initialInvest)

	// Получаем токен минт или используем дефолтный
	tokenMint := dex.tokenMint
	if tokenMint == "" {
		tokenMint = "GuKMr2mA...GSudgos9"
	}

	// Создаем строку UI
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
	pnlStr := fmt.Sprintf("-%.8f SOL (-%.2f%%)", -pnl.NetPnL, -pnl.PnLPercentage)
	if pnl.NetPnL >= 0 {
		pnlStr = fmt.Sprintf("%.8f SOL (%.2f%%)", pnl.NetPnL, pnl.PnLPercentage)
	}
	sb.WriteString(fmt.Sprintf("║ P&L:                 %-24s ║\n", pnlStr))
	sb.WriteString("╚═══════════════════════════════════════════════╝")

	return sb.String()
}

// TestRenderUIMonitor_EndToEnd тестирует полную интеграцию расчетов с UI-рендерингом
func TestRenderUIMonitor_EndToEnd(t *testing.T) {
	// 1) Подставляем RealDEXMock с реальными данными из логов
	dex := &RealDEXMock{
		price:         0.00004447, // Текущая цена токена
		balance:       0.13682400, // Количество токенов
		sellEstimate:  0.00000513, // Ожидаемая выручка от продажи
		initial:       0.00004447, // Начальная цена (равна текущей - нет изменения)
		tokenMint:     "GuKMr2mA...GSudgos9",
		initialInvest: 0.00001, // Начальная инвестиция (до учета комиссии)
		logger:        zap.NewNop(),
	}

	// 2) Вызываем RenderUIMonitor
	out := RenderRealUIMonitor(dex)

	// 3) Проверяем, что совпадает строка (с теми же цифрами из логов):
	want := `╔════════════════ TOKEN MONITOR ════════════════╗
║ Token: GuKMr2mA...GSudgos9                    ║
╟───────────────────────────────────────────────╢
║ Current Price:       0.00004447 SOL           ║
║ Initial Price:       0.00004447 SOL           ║
║ Price Change:        0.00%                    ║
║ Tokens Owned:        0.13682400               ║
╟───────────────────────────────────────────────╢
║ Sold (Estimate):     0.00000513 SOL           ║
║ Invested:            0.00000998 SOL           ║
║ P&L:                 -0.00000485 SOL (-48.57%) ║
╚═══════════════════════════════════════════════╝`

	// Для отладки - показываем разницу, если не совпадает
	if out != want {
		t.Logf("Expected:\n%s", want)
		t.Logf("Actual:\n%s", out)
		t.Logf("Length expected: %d", len(want))
		t.Logf("Length actual: %d", len(out))

		// Построчное сравнение для поиска различий
		expectedLines := strings.Split(want, "\n")
		actualLines := strings.Split(out, "\n")

		for i := 0; i < len(expectedLines) && i < len(actualLines); i++ {
			if expectedLines[i] != actualLines[i] {
				t.Logf("Mismatch at line %d:", i+1)
				t.Logf("Expected: '%s'", expectedLines[i])
				t.Logf("Actual:   '%s'", actualLines[i])
				t.Logf("Expected length: %d", len(expectedLines[i]))
				t.Logf("Actual length: %d", len(actualLines[i]))
			}
		}
	}

	assert.Equal(t, want, out)
}
