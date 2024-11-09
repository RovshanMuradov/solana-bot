# Raydium DEX Integration TODO

## Приоритет 1: Критические компоненты

### client.go
- [ ] Добавить поддержку версионированных транзакций
  ```go
  CreateVersionedSwapInstructions(ctx context.Context, params SwapParams) (*VersionedTransaction, error)
  SendVersionedTransaction(ctx context.Context, tx *VersionedTransaction) (Signature, error)
  ```
- [ ] Реализовать методы для работы с lookup tables
  ```go
  GetPoolLookupTable(ctx context.Context, pool *RaydiumPool) (*AddressLookupTable, error)
  CreateLookupTableInstruction(ctx context.Context, addresses []PublicKey) (Instruction, error)
  ```
- [ ] Добавить обработку ошибок специфичных для Raydium
  ```go
  type RaydiumError struct {
      Code    uint32
      Message string
  }
  ```

### instruction.go
- [ ] Реализовать BuildVersionedSwapInstruction
  ```go
  BuildVersionedSwapInstruction(ctx context.Context, params SwapParams) (*VersionedTransaction, error)
  ```
- [ ] Добавить поддержку CPI (Cross-Program Invocation)
  ```go
  CreateCPISwapInstruction(ctx context.Context, programId PublicKey) (Instruction, error)
  ```
- [ ] Реализовать инструкции для других операций
  ```go
  BuildDepositInstruction(ctx context.Context, params DepositParams) (Instruction, error)
  BuildWithdrawInstruction(ctx context.Context, params WithdrawParams) (Instruction, error)
  BuildInitializeInstruction(ctx context.Context, params InitParams) (Instruction, error)
  ```

### pool.go
- [ ] Добавить поддержку пулов v5
  ```go
  type PoolV5Manager struct {
      NewFeeStructure
      AmmV5Features
  }
  ```
- [ ] Реализовать методы для работы с LP токенами
  ```go
  GetLPTokenBalance(ctx context.Context, owner PublicKey) (uint64, error)
  CalculateLPTokens(amountA uint64, amountB uint64) (uint64, error)
  ```
- [ ] Добавить методы для расчета оптимальных свапов
  ```go
  GetOptimalSwapAmount(targetAmount uint64) (SwapAmount, error)
  CalculateMinimumReceived(amount uint64, slippage float64) (uint64, error)
  ```

## Приоритет 2: Улучшения функциональности

### stage.go
- [ ] Реализовать поддержку новых полей v5
  ```go
  type LayoutV5 struct {
      // Новые поля v5
      NewFeeStructure
      ExtendedFeatures
  }
  ```
- [ ] Добавить методы миграции между версиями
  ```go
  MigrateState(oldState *Layout, newVersion StateVersion) (*Layout, error)
  ValidateV5State(state *LayoutV5) error
  ```
- [ ] Реализовать обработку дополнительных флагов состояния
  ```go
  type StateFlags uint32
  ParseStateFlags(flags StateFlags) StateFeatures
  ```

### types.go
- [ ] Добавить типы для новых версий пулов
  ```go
  type RaydiumPoolV5 struct {
      // Новая структура пула v5
  }
  ```
- [ ] Реализовать структуры для маркет-мейкинга
  ```go
  type MarketMakingParams struct {
      // Параметры MM
  }
  ```
- [ ] Добавить типы для расширенных операций
  ```go
  type AdvancedSwapParams struct {
      // Расширенные параметры свапа
  }
  ```

### utils.go
- [ ] Добавить утилиты для версионированных транзакций
  ```go
  CreateVersionedTransaction(instructions []Instruction, lookupTables []AddressLookupTable) (*VersionedTransaction, error)
  SimulateVersionedTransaction(tx *VersionedTransaction) (SimulationResponse, error)
  ```
- [ ] Реализовать методы для работы с новыми типами комиссий
  ```go
  CalculateV5Fees(amount uint64, feeParams V5FeeParams) (uint64, error)
  ParseFeeStructure(data []byte) (FeeParams, error)
  ```
- [ ] Добавить хелперы для работы с lookup tables
  ```go
  CreateLookupTable(addresses []PublicKey) (AddressLookupTable, error)
  UpdateLookupTable(table AddressLookupTable, newAddresses []PublicKey) error
  ```

## Приоритет 3: Оптимизации и улучшения

### Метрики и мониторинг
- [ ] Добавить Prometheus метрики
  ```go
  // metrics.go
  var (
      SwapLatency prometheus.Histogram
      PoolLiquidity prometheus.Gauge
      TransactionErrors prometheus.Counter
  )
  ```

### Тестирование
- [ ] Добавить интеграционные тесты
  ```go
  // integration_test.go
  TestRaydiumSwap(t *testing.T)
  TestPoolInitialization(t *testing.T)
  TestVersionedTransactions(t *testing.T)
  ```
- [ ] Реализовать моки для тестирования
  ```go
  // mocks.go
  type MockRaydiumClient struct {}
  type MockPoolManager struct {}
  ```

### Документация
- [ ] Создать godoc документацию для всех публичных методов
- [ ] Добавить примеры использования
- [ ] Создать руководство по миграции на v5

### Оптимизации
- [ ] Реализовать кеширование состояния пула
- [ ] Оптимизировать работу с транзакциями
- [ ] Улучшить обработку ошибок и retry логику

## Зависимости и требования

### Внешние зависимости
- solana-go v1.8.0 или выше
- zap logger
- prometheus client
- testify для тестирования

### Системные требования
- Go 1.19 или выше
- Поддержка контекстов
- Работа с большими числами

## Порядок реализации

1. Начать с критических компонентов в client.go и instruction.go
2. Реализовать поддержку v5 в pool.go и stage.go
3. Добавить новые типы и структуры в types.go
4. Реализовать утилиты в utils.go
5. Добавить метрики и мониторинг
6. Написать тесты
7. Подготовить документацию

## Замечания по реализации

- Все новые методы должны поддерживать контекст
- Использовать zap logger для логирования
- Добавлять метрики для каждой критической операции
- Обеспечить обратную совместимость
- Следовать паттернам обработки ошибок из solana-go

## Метрики успеха

- [ ] 100% покрытие тестами критического функционала
- [ ] Успешная обработка всех типов транзакций
- [ ] Поддержка всех версий пулов
- [ ] Полная совместимость с TypeScript SDK
- [ ] Документация для всех публичных API