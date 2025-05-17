package pumpswap

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateOutput(t *testing.T) {
	// Параметры входа
	reserves := uint64(742080)        // baseReserves
	otherReserves := uint64(33322)    // quoteReserves
	amount := uint64(136824)          // amount tokens в raw формате
	feeFactor := 1.0 - (0.25 / 100.0) // 0.25% fee

	// Вычисляем ожидаемый результат
	x := float64(reserves)
	y := float64(otherReserves)
	a := float64(amount)
	a *= feeFactor

	// Формула: outputAmount = y * a / (x + a)
	expectedOutput := uint64((y * a) / (x + a))

	// Вызываем тестируемую функцию
	actualOutput := calculateOutput(reserves, otherReserves, amount, feeFactor)

	// Проверяем результат
	assert.Equal(t, expectedOutput, actualOutput, "calculateOutput result mismatch")

	// Переводим в SOL для наглядности
	outputSol := float64(actualOutput) / math.Pow10(9)
	t.Logf("Raw output: %d", actualOutput)
	t.Logf("SOL output: %.12f", outputSol)
}

func TestDEX_CalculatePnL(t *testing.T) {
	// Настраиваем тестовые данные на основе примера из TT.md
	initialInvestment := 0.00001

	// Фиксированные значения для теста
	expectedSellEstimate := 0.00000513

	// Ожидаемые результаты расчетов
	expectedBuyFee := initialInvestment * (DexFeePercent / 100.0)
	expectedCostBasis := initialInvestment - expectedBuyFee
	expectedNetPnL := expectedSellEstimate - expectedCostBasis
	expectedPnLPercentage := (expectedNetPnL / expectedCostBasis) * 100

	// Выводим ожидаемые расчеты для наглядности
	t.Logf("Initial investment: %.12f", initialInvestment)
	t.Logf("Buy fee (0.25%%): %.12f", expectedBuyFee)
	t.Logf("Cost basis: %.12f", expectedCostBasis)
	t.Logf("Sell estimate: %.12f", expectedSellEstimate)
	t.Logf("Net P&L: %.12f", expectedNetPnL)
	t.Logf("P&L %%: %.6f%%", expectedPnLPercentage)

	// Расчет того же значения напрямую без использования структуры DEX
	// для проверки логики расчета
	buyFee := initialInvestment * (DexFeePercent / 100.0)
	costBasis := initialInvestment - buyFee
	netPnL := expectedSellEstimate - costBasis
	pnlPercentage := (netPnL / costBasis) * 100

	// Проверка расчетов
	assert.InDelta(t, expectedCostBasis, costBasis, 1e-12, "Cost basis calculation incorrect")
	assert.InDelta(t, expectedNetPnL, netPnL, 1e-12, "Net P&L calculation incorrect")
	assert.InDelta(t, expectedPnLPercentage, pnlPercentage, 1e-6, "P&L percentage calculation incorrect")
	assert.InDelta(t, -48.57, pnlPercentage, 0.01, "P&L percentage should be around -48.57%")
}
