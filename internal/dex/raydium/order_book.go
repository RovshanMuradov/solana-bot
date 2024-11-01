package raydium

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/big"
	"time"

	"github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/blockchain" // internal/dex/raydium/order_book.go
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// TODO: order_book.go:
// - Упростить структуру, использовать более эффективные структуры данных
// - Добавить методы для работы с orderbook snapshots

// Константы для Serum OrderBook
const (
	ORDER_BOOK_LAYOUT_SIZE = 5132
	SLAB_LAYOUT_SIZE       = 64
	EVENT_QUEUE_SIZE       = 1024

	ASKS_SIZE = ORDER_BOOK_LAYOUT_SIZE
	BIDS_SIZE = ORDER_BOOK_LAYOUT_SIZE
)

// OrderSide определяет сторону ордера
type OrderSide uint8

const (
	Bid OrderSide = iota
	Ask
)

// OrderType определяет тип ордера
type OrderType uint8

const (
	Limit OrderType = iota
	ImmediateOrCancel
	PostOnly
)

// Order представляет ордер в книге
type Order struct {
	Side      OrderSide
	OrderType OrderType
	OrderID   *big.Int
	Owner     solana.PublicKey
	Price     decimal.Decimal
	Size      uint64
	ClientID  uint64
}

// OrderBookSide представляет одну сторону книги ордеров (бид или аск)
type OrderBookSide struct {
	Slab []Order
}

// OrderBook управляет книгой ордеров
type OrderBook struct {
	client  blockchain.Client
	logger  *zap.Logger
	market  solana.PublicKey
	program solana.PublicKey

	bids        solana.PublicKey
	asks        solana.PublicKey
	eventQueue  solana.PublicKey
	baseVault   solana.PublicKey
	quoteVault  solana.PublicKey
	vaultSigner solana.PublicKey

	bidsSide OrderBookSide
	asksSide OrderBookSide

	cache    *OrderBookCache
	cacheTTL time.Duration
}

// NewOrderBook создает новый экземпляр OrderBook
func NewOrderBook(
	client blockchain.Client,
	logger *zap.Logger,
	market solana.PublicKey,
	program solana.PublicKey,
) *OrderBook {
	return &OrderBook{
		client:  client,
		logger:  logger.Named("order-book"),
		market:  market,
		program: program,
	}
}

// Initialize инициализирует OrderBook и загружает начальные данные
func (ob *OrderBook) Initialize(ctx context.Context) error {
	logger := ob.logger.With(
		zap.String("market", ob.market.String()),
		zap.String("program", ob.program.String()),
	)
	logger.Debug("Initializing order book")

	// Получаем информацию о маркете
	marketInfo, err := ob.client.GetAccountInfo(ctx, ob.market)
	if err != nil {
		return fmt.Errorf("failed to get market info: %w", err)
	}

	// Декодируем данные маркета
	if err := ob.decodeMarketData(marketInfo.Value.Data.GetBinary()); err != nil {
		return fmt.Errorf("failed to decode market data: %w", err)
	}

	// Загружаем начальное состояние ордербука
	if err := ob.loadOrderBook(ctx); err != nil {
		return fmt.Errorf("failed to load order book: %w", err)
	}

	return nil
}

// LoadOrderBook загружает текущее состояние книги ордеров
func (ob *OrderBook) loadOrderBook(ctx context.Context) error {
	// Загружаем биды
	if err := ob.loadBids(ctx); err != nil {
		return fmt.Errorf("failed to load bids: %w", err)
	}

	// Загружаем аски
	if err := ob.loadAsks(ctx); err != nil {
		return fmt.Errorf("failed to load asks: %w", err)
	}

	return nil
}

// GetBestBid возвращает лучший бид
func (ob *OrderBook) GetBestBid() (*Order, error) {
	if len(ob.bidsSide.Slab) == 0 {
		return nil, fmt.Errorf("no bids available")
	}
	return &ob.bidsSide.Slab[0], nil
}

// GetBestAsk возвращает лучший аск
func (ob *OrderBook) GetBestAsk() (*Order, error) {
	if len(ob.asksSide.Slab) == 0 {
		return nil, fmt.Errorf("no asks available")
	}
	return &ob.asksSide.Slab[0], nil
}

// GetSpread возвращает текущий спред
func (ob *OrderBook) GetSpread() (decimal.Decimal, error) {
	bestBid, err := ob.GetBestBid()
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to get best bid: %w", err)
	}

	bestAsk, err := ob.GetBestAsk()
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to get best ask: %w", err)
	}

	return bestAsk.Price.Sub(bestBid.Price), nil
}

// ProcessEvent обрабатывает событие из очереди
type Event struct {
	EventType uint8
	Slot      uint64
	Order     Order
	FillSize  uint64
	Fee       uint64
	Timestamp time.Time
}

func (ob *OrderBook) ProcessEvent(ctx context.Context, event Event) error {
	logger := ob.logger.With(
		zap.Uint8("event_type", event.EventType),
		zap.Uint64("slot", event.Slot),
		zap.String("order_id", event.Order.OrderID.String()),
	)
	logger.Debug("Processing event")

	switch event.EventType {
	case 0: // Fill
		return ob.processFill(event)
	case 1: // Out
		return ob.processOut(event)
	default:
		return fmt.Errorf("unknown event type: %d", event.EventType)
	}
}

// ProcessEventQueue обрабатывает очередь событий
func (ob *OrderBook) ProcessEventQueue(ctx context.Context) error {
	ob.logger.Debug("Processing event queue")

	// Получаем данные очереди событий
	queueData, err := ob.client.GetAccountInfo(ctx, ob.eventQueue)
	if err != nil {
		return fmt.Errorf("failed to get event queue data: %w", err)
	}

	// Декодируем и обрабатываем события
	events, err := ob.decodeEventQueue(queueData.Value.Data.GetBinary())
	if err != nil {
		return fmt.Errorf("failed to decode event queue: %w", err)
	}

	for _, event := range events {
		if err := ob.ProcessEvent(ctx, event); err != nil {
			ob.logger.Error("Failed to process event",
				zap.Error(err),
				zap.Uint8("event_type", event.EventType),
			)
			continue
		}
	}

	return nil
}

// Вспомогательные методы

func (ob *OrderBook) decodeMarketData(data []byte) error {
	if len(data) < 8 {
		return fmt.Errorf("invalid market data length")
	}

	offset := 8 // Пропускаем discriminator

	// Читаем адреса ордербука
	ob.bids = solana.PublicKeyFromBytes(data[offset : offset+32])
	offset += 32

	ob.asks = solana.PublicKeyFromBytes(data[offset : offset+32])
	offset += 32

	ob.eventQueue = solana.PublicKeyFromBytes(data[offset : offset+32])
	offset += 32

	ob.baseVault = solana.PublicKeyFromBytes(data[offset : offset+32])
	offset += 32

	ob.quoteVault = solana.PublicKeyFromBytes(data[offset : offset+32])
	offset += 32

	return nil
}

func (ob *OrderBook) loadBids(ctx context.Context) error {
	data, err := ob.client.GetAccountInfo(ctx, ob.bids)
	if err != nil {
		return fmt.Errorf("failed to get bids data: %w", err)
	}

	ob.bidsSide.Slab = make([]Order, 0)
	return ob.decodeSlab(data.Value.Data.GetBinary(), &ob.bidsSide.Slab)
}

func (ob *OrderBook) loadAsks(ctx context.Context) error {
	data, err := ob.client.GetAccountInfo(ctx, ob.asks)
	if err != nil {
		return fmt.Errorf("failed to get asks data: %w", err)
	}

	ob.asksSide.Slab = make([]Order, 0)
	return ob.decodeSlab(data.Value.Data.GetBinary(), &ob.asksSide.Slab)
}

func (ob *OrderBook) decodeSlab(data []byte, orders *[]Order) error {
	if len(data) < SLAB_LAYOUT_SIZE {
		return fmt.Errorf("invalid slab data length")
	}

	// Декодируем заголовок слаба
	bumpIndex := binary.LittleEndian.Uint32(data[0:4])
	freeListLen := binary.LittleEndian.Uint32(data[4:8])
	freeListHead := binary.LittleEndian.Uint32(data[8:12])

	// Вычисляем количество активных нодов
	activeNodes := bumpIndex - freeListLen

	// Пропускаем ноды из free list
	freeList := make(map[uint32]bool)
	offset := SLAB_LAYOUT_SIZE

	// Собираем free list
	current := freeListHead
	for i := uint32(0); i < freeListLen; i++ {
		freeList[current] = true
		// Получаем следующий свободный индекс
		if offset+4 <= len(data) {
			current = binary.LittleEndian.Uint32(data[offset : offset+4])
			offset += 4
		}
	}

	// Резервируем слайс под активные ноды
	*orders = make([]Order, 0, activeNodes)

	// Теперь декодируем только активные ноды
	for i := uint32(0); i < activeNodes; i++ {
		// Пропускаем ноды из free list
		if freeList[i] {
			continue
		}

		var order Order
		// Декодируем каждый ордер
		order.OrderID = new(big.Int).SetUint64(binary.LittleEndian.Uint64(data[offset : offset+8]))
		offset += 8

		copy(order.Owner[:], data[offset:offset+32])
		offset += 32

		priceRaw := binary.LittleEndian.Uint64(data[offset : offset+8])
		order.Price = decimal.NewFromInt(int64(priceRaw))
		offset += 8

		order.Size = binary.LittleEndian.Uint64(data[offset : offset+8])
		offset += 8

		order.ClientID = binary.LittleEndian.Uint64(data[offset : offset+8])
		offset += 8

		*orders = append(*orders, order)
	}

	// Проверяем, что мы декодировали правильное количество ордеров
	if uint32(len(*orders)) != activeNodes {
		return fmt.Errorf("decoded orders count (%d) doesn't match active nodes count (%d)", len(*orders), activeNodes)
	}

	return nil
}

func (ob *OrderBook) decodeEventQueue(data []byte) ([]Event, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("invalid event queue data length")
	}

	head := binary.LittleEndian.Uint32(data[0:4])
	count := binary.LittleEndian.Uint32(data[4:8])
	events := make([]Event, 0, count)

	// Размер одного события
	eventSize := 57 // 1 + 8 + 8 + 32 + 8 + 8 bytes

	// Начинаем с head и идем по кругу
	for i := uint32(0); i < count; i++ {
		// Вычисляем позицию текущего события с учетом закольцованности
		position := (head + i) % uint32(EVENT_QUEUE_SIZE)
		offset := 8 + position*uint32(eventSize)

		var event Event

		event.EventType = data[offset]
		offset += 1

		event.Slot = binary.LittleEndian.Uint64(data[offset : offset+8])
		offset += 8

		event.Order.OrderID = new(big.Int).SetUint64(binary.LittleEndian.Uint64(data[offset : offset+8]))
		offset += 8

		copy(event.Order.Owner[:], data[offset:offset+32])
		offset += 32

		event.FillSize = binary.LittleEndian.Uint64(data[offset : offset+8])
		offset += 8

		event.Fee = binary.LittleEndian.Uint64(data[offset : offset+8])
		offset += 8

		events = append(events, event)
	}

	return events, nil
}

func (ob *OrderBook) processFill(event Event) error {
	// Обновляем состояние ордербука при заполнении ордера
	if event.Order.Side == Bid {
		return ob.updateBidSide(event.Order.OrderID, event.FillSize)
	}
	return ob.updateAskSide(event.Order.OrderID, event.FillSize)
}

func (ob *OrderBook) processOut(event Event) error {
	// Обрабатываем удаление ордера
	if event.Order.Side == Bid {
		return ob.removeBid(event.Order.OrderID)
	}
	return ob.removeAsk(event.Order.OrderID)
}

func (ob *OrderBook) updateBidSide(orderID *big.Int, fillSize uint64) error {
	for i := range ob.bidsSide.Slab {
		if ob.bidsSide.Slab[i].OrderID == orderID {
			ob.bidsSide.Slab[i].Size -= fillSize
			if ob.bidsSide.Slab[i].Size == 0 {
				// Удаляем полностью заполненный ордер
				ob.bidsSide.Slab = append(ob.bidsSide.Slab[:i], ob.bidsSide.Slab[i+1:]...)
			}
			return nil
		}
	}
	return fmt.Errorf("bid order not found: %s", orderID)
}

func (ob *OrderBook) updateAskSide(orderID *big.Int, fillSize uint64) error {
	for i := range ob.asksSide.Slab {
		if ob.asksSide.Slab[i].OrderID == orderID {
			ob.asksSide.Slab[i].Size -= fillSize
			if ob.asksSide.Slab[i].Size == 0 {
				// Удаляем полностью заполненный ордер
				ob.asksSide.Slab = append(ob.asksSide.Slab[:i], ob.asksSide.Slab[i+1:]...)
			}
			return nil
		}
	}
	return fmt.Errorf("ask order not found: %s", orderID)
}

func (ob *OrderBook) removeBid(orderID *big.Int) error {
	for i := range ob.bidsSide.Slab {
		if ob.bidsSide.Slab[i].OrderID == orderID {
			ob.bidsSide.Slab = append(ob.bidsSide.Slab[:i], ob.bidsSide.Slab[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("bid order not found: %s", orderID)
}

func (ob *OrderBook) removeAsk(orderID *big.Int) error {
	for i := range ob.asksSide.Slab {
		if ob.asksSide.Slab[i].OrderID == orderID {
			ob.asksSide.Slab = append(ob.asksSide.Slab[:i], ob.asksSide.Slab[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("ask order not found: %s", orderID)
}

// GetOrderBookDepth возвращает глубину книги ордеров до определенного уровня
func (ob *OrderBook) GetOrderBookDepth(levels int) (bids []Order, asks []Order) {
	bids = make([]Order, 0, levels)
	asks = make([]Order, 0, levels)

	// Копируем биды до указанной глубины
	for i := 0; i < len(ob.bidsSide.Slab) && i < levels; i++ {
		bids = append(bids, ob.bidsSide.Slab[i])
	}

	// Копируем аски до указанной глубины
	for i := 0; i < len(ob.asksSide.Slab) && i < levels; i++ {
		asks = append(asks, ob.asksSide.Slab[i])
	}

	return bids, asks
}

// GetVolumeAtPrice возвращает объем ордеров по заданной цене
func (ob *OrderBook) GetVolumeAtPrice(price decimal.Decimal, side OrderSide) uint64 {
	var volume uint64

	if side == Bid {
		for _, order := range ob.bidsSide.Slab {
			if order.Price.Equal(price) {
				volume += order.Size
			}
		}
	} else {
		for _, order := range ob.asksSide.Slab {
			if order.Price.Equal(price) {
				volume += order.Size
			}
		}
	}

	return volume
}

// GetTotalVolume возвращает общий объем ордеров для каждой стороны
func (ob *OrderBook) GetTotalVolume() (bidVolume uint64, askVolume uint64) {
	for _, order := range ob.bidsSide.Slab {
		bidVolume += order.Size
	}

	for _, order := range ob.asksSide.Slab {
		askVolume += order.Size
	}

	return bidVolume, askVolume
}

// FindOrder ищет ордер по ID
func (ob *OrderBook) FindOrder(orderID *big.Int) (*Order, OrderSide, error) {
	// Поиск в бидах
	for i := range ob.bidsSide.Slab {
		if ob.bidsSide.Slab[i].OrderID == orderID {
			return &ob.bidsSide.Slab[i], Bid, nil
		}
	}

	// Поиск в асках
	for i := range ob.asksSide.Slab {
		if ob.asksSide.Slab[i].OrderID == orderID {
			return &ob.asksSide.Slab[i], Ask, nil
		}
	}

	return nil, 0, fmt.Errorf("order not found: %s", orderID)
}

// GetOrdersForOwner возвращает все ордера для указанного владельца
func (ob *OrderBook) GetOrdersForOwner(owner solana.PublicKey) []Order {
	orders := make([]Order, 0)

	// Поиск в бидах
	for _, order := range ob.bidsSide.Slab {
		if order.Owner.Equals(owner) {
			orders = append(orders, order)
		}
	}

	// Поиск в асках
	for _, order := range ob.asksSide.Slab {
		if order.Owner.Equals(owner) {
			orders = append(orders, order)
		}
	}

	return orders
}

// OrderBookCache представляет кеш состояния ордербука
type OrderBookCache struct {
	LastUpdate time.Time
	Bids       []Order
	Asks       []Order
}

// UpdateCache обновляет кеш ордербука
func (ob *OrderBook) UpdateCache(ctx context.Context) error {
	if ob.cache == nil || time.Since(ob.cache.LastUpdate) > ob.cacheTTL {
		if err := ob.loadOrderBook(ctx); err != nil {
			return err
		}

		ob.cache = &OrderBookCache{
			LastUpdate: time.Now(),
			Bids:       append([]Order{}, ob.bidsSide.Slab...),
			Asks:       append([]Order{}, ob.asksSide.Slab...),
		}
	}
	return nil
}
