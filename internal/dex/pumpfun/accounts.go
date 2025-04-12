// =============================
// File: internal/dex/pumpfun/accounts.go
// =============================
package pumpfun

import (
	"context"
	"encoding/binary"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain/solbc"
	"go.uber.org/zap"
	"strconv"
)

// deriveBondingCurveAccounts вычисляет необходимые адреса для взаимодействия
// с bonding curve токена в протоколе Pump.fun.
func (d *DEX) deriveBondingCurveAccounts(_ context.Context) (bondingCurve, associatedBondingCurve solana.PublicKey, err error) {
	// Шаг 1: Вычисление Program Derived Address (PDA) для bonding curve
	bondingCurve, _, err = solana.FindProgramAddress(
		[][]byte{[]byte("bonding-curve"), d.config.Mint.Bytes()},
		d.config.ContractAddress,
	)

	// Шаг 2: Проверка на ошибки при вычислении PDA
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("failed to derive bonding curve: %w", err)
	}

	// Шаг 3: Логирование успешного вычисления адреса bonding curve для отладки
	d.logger.Debug("Derived bonding curve", zap.String("address", bondingCurve.String()))

	// Шаг 4: Вычисление ассоциированного токен-аккаунта (ATA) для bonding curve
	associatedBondingCurve, _, err = solana.FindAssociatedTokenAddress(bondingCurve, d.config.Mint)

	// Шаг 5: Проверка на ошибки при вычислении ATA
	if err != nil {
		return solana.PublicKey{}, solana.PublicKey{}, fmt.Errorf("failed to derive associated bonding curve: %w", err)
	}

	// Шаг 6: Логирование успешного вычисления адреса ATA для отладки
	d.logger.Debug("Derived bonding curve ATA", zap.String("address", associatedBondingCurve.String()))

	// Шаг 7: Возврат вычисленных адресов
	return bondingCurve, associatedBondingCurve, nil
}

// FetchBondingCurveAccount получает и парсит данные аккаунта bonding curve.
func (d *DEX) FetchBondingCurveAccount(ctx context.Context, bondingCurve solana.PublicKey) (*BondingCurve, error) {
	// Шаг 1: Получение информации об аккаунте с блокчейна
	accountInfo, err := d.client.GetAccountInfo(ctx, bondingCurve)
	if err != nil {
		// Шаг 2: Обработка ошибки при неудачном запросе
		return nil, fmt.Errorf("failed to get bonding curve account: %w", err)
	}

	// Шаг 3: Проверка существования аккаунта
	if accountInfo.Value == nil {
		return nil, fmt.Errorf("bonding curve account not found")
	}

	// Шаг 4: Извлечение бинарных данных из аккаунта
	data := accountInfo.Value.Data.GetBinary()

	// Шаг 5: Проверка минимальной длины данных
	if len(data) < 16 {
		return nil, fmt.Errorf("invalid bonding curve data: insufficient length")
	}

	// Шаг 6: Чтение виртуальных резервов токенов (первые 8 байт)
	virtualTokenReserves := binary.LittleEndian.Uint64(data[0:8]) // TODO: work with virtual balance

	// Шаг 7: Чтение виртуальных резервов SOL (следующие 8 байт)
	virtualSolReserves := binary.LittleEndian.Uint64(data[8:16])

	// Шаг 8: Создание и возврат структуры с данными
	return &BondingCurve{
		VirtualTokenReserves: virtualTokenReserves,
		VirtualSolReserves:   virtualSolReserves,
	}, nil
}

// FetchGlobalAccount получает и парсит данные глобального аккаунта Pump.fun.
func FetchGlobalAccount(ctx context.Context, client *solbc.Client, globalAddr solana.PublicKey) (*GlobalAccount, error) {
	// Шаг 1: Получение информации об аккаунте с блокчейна
	accountInfo, err := client.GetAccountInfo(ctx, globalAddr)
	if err != nil {
		// Шаг 2: Обработка ошибки при неудачном запросе
		return nil, fmt.Errorf("failed to get global account: %w", err)
	}

	// Шаг 3: Проверка существования аккаунта
	if accountInfo == nil || accountInfo.Value == nil {
		return nil, fmt.Errorf("global account not found: %s", globalAddr.String())
	}

	// Шаг 4: Проверка владельца аккаунта
	if !accountInfo.Value.Owner.Equals(PumpFunProgramID) {
		return nil, fmt.Errorf("global account has incorrect owner: expected %s, got %s",
			PumpFunProgramID.String(), accountInfo.Value.Owner.String())
	}

	// Шаг 5: Извлечение бинарных данных из аккаунта
	data := accountInfo.Value.Data.GetBinary()

	// Шаг 6: Проверка достаточности данных
	// Минимальная длина: 8 (дискриминатор) + 1 (флаг) + 64 (два публичных ключа)
	if len(data) < 8+1+64 {
		return nil, fmt.Errorf("global account data too short: %d bytes", len(data))
	}

	// Шаг 7: Начало десериализации - создание структуры
	account := &GlobalAccount{}

	// Шаг 8: Чтение дискриминатора (8 байт)
	copy(account.Discriminator[:], data[0:8])

	// Шаг 9: Чтение флага инициализации (1 байт)
	account.Initialized = data[8] != 0

	// Шаг 10: Чтение публичного ключа администратора (32 байта)
	authorityBytes := make([]byte, 32)
	copy(authorityBytes, data[9:41])
	account.Authority = solana.PublicKeyFromBytes(authorityBytes)

	// Шаг 11: Чтение публичного ключа получателя комиссий (32 байта)
	feeRecipientBytes := make([]byte, 32)
	copy(feeRecipientBytes, data[41:73])
	account.FeeRecipient = solana.PublicKeyFromBytes(feeRecipientBytes)

	// Шаг 12: Чтение размера комиссии в базовых пунктах (8 байт)
	if len(data) >= 81 {
		account.FeeBasisPoints = binary.LittleEndian.Uint64(data[73:81])
	}

	// Шаг 13: Возврат заполненной структуры
	return account, nil
}

// GetTokenPrice возвращает текущую цену токена на Pump.fun, используя данные bonding curve.
func (d *DEX) GetTokenPrice(ctx context.Context, tokenMint string) (float64, error) {
	// Проверка соответствия запрашиваемого токена настроенному в DEX
	if d.config.Mint.String() != tokenMint {
		return 0, fmt.Errorf("token %s not configured in this DEX instance", tokenMint)
	}

	// Вычисление адреса bonding curve для токена
	bondingCurve, _, err := solana.FindProgramAddress(
		[][]byte{[]byte("bonding-curve"), d.config.Mint.Bytes()},
		d.config.ContractAddress,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to derive bonding curve: %w", err)
	}

	// Получение данных аккаунта bonding curve с менее строгим уровнем подтверждения
	// для более быстрого доступа к новым токенам
	accountInfo, err := d.client.GetAccountInfo(ctx, bondingCurve)

	// Если аккаунт не найден, возможно, это очень новый токен
	if err != nil || accountInfo == nil || accountInfo.Value == nil {
		// Для новых токенов возвращаем минимальную цену
		d.logger.Warn("Bonding curve account not found or error, assuming new token with minimal price",
			zap.String("token_mint", tokenMint),
			zap.Error(err))
		return 0.000000001, nil // Минимальная цена для новых токенов
	}

	// Парсим данные bonding curve
	bondingCurveData, err := d.FetchBondingCurveAccount(ctx, bondingCurve)
	if err != nil {
		d.logger.Warn("Failed to parse bonding curve data, assuming new token",
			zap.String("token_mint", tokenMint),
			zap.Error(err))
		return 0.000000001, nil
	}

	// Проверка на нулевые резервы (характерно для очень новых токенов)
	if bondingCurveData.VirtualTokenReserves == 0 || bondingCurveData.VirtualSolReserves == 0 {
		d.logger.Warn("Bonding curve has zero reserves, assuming new token",
			zap.String("token_mint", tokenMint))
		return 0.000000001, nil
	}

	// Используем общую функцию для расчета цены токена
	return d.CalculateTokenPrice(ctx, bondingCurveData)
}

// GetTokenBalance возвращает текущий баланс токена в кошельке пользователя.
// Метод определяет ассоциированный токен-аккаунт для кошелька и запрашивает его баланс.
// Сначала пытается получить баланс с использованием быстрого уровня подтверждения Processed,
// при неудаче переключается на более надежный уровень Confirmed.
func (d *DEX) GetTokenBalance(ctx context.Context, commitment ...rpc.CommitmentType) (uint64, error) {
	// Шаг 1: Вычисление адреса ассоциированного токен-аккаунта (ATA)
	// ATA - стандартизированный адрес для хранения SPL-токенов, уникальный для пары (владелец, минт)
	userATA, _, err := solana.FindAssociatedTokenAddress(d.wallet.PublicKey, d.config.Mint)
	if err != nil {
		return 0, fmt.Errorf("failed to derive associated token account: %w", err)
	}

	// Шаг 2: Определение уровня подтверждения (commitment level)
	// По умолчанию используем Processed - самый быстрый уровень
	// Можно переопределить через вариативный параметр
	commitmentLevel := rpc.CommitmentProcessed
	if len(commitment) > 0 && commitment[0] != "" {
		commitmentLevel = commitment[0]
	}

	// Шаг 3: Запрос баланса токена с выбранным уровнем подтверждения
	result, err := d.client.GetTokenAccountBalance(ctx, userATA, commitmentLevel)

	// Шаг 4: При неудаче с Processed, пробуем Confirmed
	if err != nil && commitmentLevel == rpc.CommitmentProcessed {
		d.logger.Debug("Failed to get balance with Processed commitment, trying Confirmed",
			zap.String("token_mint", d.config.Mint.String()),
			zap.String("user_ata", userATA.String()),
			zap.Error(err))

		// Повторный запрос с более надежным уровнем подтверждения
		result, err = d.client.GetTokenAccountBalance(ctx, userATA, rpc.CommitmentConfirmed)
	}

	// Если ошибка все еще присутствует, возвращаем ее
	if err != nil {
		return 0, fmt.Errorf("failed to get token account balance: %w", err)
	}

	// Шаг 5: Проверка результата на пустоту
	// Убеждаемся, что получены корректные данные
	if result == nil || result.Value.Amount == "" {
		return 0, fmt.Errorf("no token balance found")
	}

	// Шаг 6: Преобразование строкового представления баланса в uint64
	// SPL-токены в Solana представлены как строки для поддержки больших чисел
	balance, err := strconv.ParseUint(result.Value.Amount, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse token balance: %w", err)
	}

	// Шаг 7: Логирование для отладки
	d.logger.Debug("Got token balance",
		zap.Uint64("balance", balance),
		zap.String("token_mint", d.config.Mint.String()),
		zap.String("user_ata", userATA.String()),
		zap.String("commitment", string(commitmentLevel)))

	// Шаг 8: Возврат полученного баланса
	return balance, nil
}
