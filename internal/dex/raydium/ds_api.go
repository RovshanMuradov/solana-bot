// internal/dex/raydium/ds_api.go

package raydium

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go"
	"go.uber.org/zap"
)

const (
	baseURL     = "https://api.dexscreener.com/latest/dex"
	rateLimit   = 300 // requests per minute
	solanaChain = "solana"
	raydiumDex  = "raydium"
)

// DexScreenerResponse представляет основную структуру ответа
type DexScreenerResponse struct {
	SchemaVersion string     `json:"schemaVersion"`
	Pairs         []PairInfo `json:"pairs"`
}

// PairInfo содержит информацию о паре
type PairInfo struct {
	ChainId       string        `json:"chainId"`
	DexId         string        `json:"dexId"`
	PairAddress   string        `json:"pairAddress"`
	BaseToken     TokenInfo     `json:"baseToken"`
	QuoteToken    TokenInfo     `json:"quoteToken"`
	PriceNative   string        `json:"priceNative"`
	Liquidity     LiquidityInfo `json:"liquidity"`
	PairCreatedAt int64         `json:"pairCreatedAt"`
}

// TokenInfo содержит информацию о токене
type TokenInfo struct {
	Address string `json:"address"`
	Symbol  string `json:"symbol"`
}

// LiquidityInfo содержит информацию о ликвидности
type LiquidityInfo struct {
	USD   float64 `json:"usd"`
	Base  float64 `json:"base"`
	Quote float64 `json:"quote"`
}

// Service представляет сервис для работы с DexScreener API
type Service struct {
	client      *http.Client
	logger      *zap.Logger
	rateLimiter *time.Ticker
	mu          sync.Mutex
}

// NewService создает новый экземпляр сервиса
func NewService(logger *zap.Logger) *Service {
	return &Service{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger:      logger.Named("dexscreener"),
		rateLimiter: time.NewTicker(time.Minute / rateLimit),
	}
}

// GetPoolByToken получает информацию о пуле по токену
func (s *Service) GetPoolByToken(ctx context.Context, tokenMint solana.PublicKey, wsolAddress string) (*PairInfo, error) {
	url := fmt.Sprintf("%s/tokens/%s", baseURL, tokenMint.String())

	response, err := s.doRequest(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to get token pairs: %w", err)
	}

	// Ищем Raydium пару с WSOL с наибольшей ликвидностью
	var bestPair *PairInfo
	maxLiquidity := 0.0

	for i := range response.Pairs {
		pair := &response.Pairs[i]

		// Строгая проверка на Raydium
		if pair.DexId != raydiumDex {
			continue
		}

		// Проверяем сеть
		if pair.ChainId != solanaChain {
			continue
		}

		// Проверяем наличие WSOL в паре
		if pair.BaseToken.Address != wsolAddress && pair.QuoteToken.Address != wsolAddress {
			continue
		}

		// Проверяем ликвидность
		if pair.Liquidity.USD > maxLiquidity {
			maxLiquidity = pair.Liquidity.USD
			bestPair = pair
		}
	}

	if bestPair == nil {
		return nil, fmt.Errorf("no Raydium/WSOL pair found for token %s", tokenMint.String())
	}

	s.logger.Info("found Raydium pair",
		zap.String("pair_address", bestPair.PairAddress),
		zap.String("base_token", bestPair.BaseToken.Symbol),
		zap.String("quote_token", bestPair.QuoteToken.Symbol),
		zap.Float64("liquidity_usd", maxLiquidity),
		zap.String("dex", bestPair.DexId)) // Добавляем лог DEX для проверки

	return bestPair, nil
}

// MonitorPair отслеживает состояние конкретной пары
func (s *Service) MonitorPair(ctx context.Context, pairAddress string) (*PairInfo, error) {
	url := fmt.Sprintf("%s/pairs/%s/%s", baseURL, solanaChain, pairAddress)

	response, err := s.doRequest(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to monitor pair: %w", err)
	}

	if len(response.Pairs) == 0 {
		return nil, fmt.Errorf("pair not found: %s", pairAddress)
	}

	return &response.Pairs[0], nil
}

// doRequest выполняет HTTP запрос с учетом rate limit
func (s *Service) doRequest(ctx context.Context, url string) (*DexScreenerResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.rateLimiter.C:
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var response DexScreenerResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &response, nil
}

// PriceMonitor отслеживает изменение цены токена относительно SOL
type PriceMonitor struct {
	pairAddress     string
	initialPrice    float64
	updateInterval  time.Duration
	priceChangeChan chan PriceUpdate
	stopChan        chan struct{}
	service         *Service
	logger          *zap.Logger
}

type PriceUpdate struct {
	PriceInSol      float64
	PriceChangePerc float64
	LiquidityInSol  float64
	UpdateTime      time.Time
}

// StartPriceMonitoring запускает мониторинг цены
func (s *Service) StartPriceMonitoring(ctx context.Context, pairAddress string, initialPriceInSol float64) (*PriceMonitor, error) {
	monitor := &PriceMonitor{
		pairAddress:     pairAddress,
		initialPrice:    initialPriceInSol,
		updateInterval:  5 * time.Second,
		priceChangeChan: make(chan PriceUpdate, 1),
		stopChan:        make(chan struct{}),
		service:         s,
		logger:          s.logger.Named("price-monitor"),
	}

	go monitor.run(ctx)
	return monitor, nil
}

func (m *PriceMonitor) run(ctx context.Context) {
	ticker := time.NewTicker(m.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopChan:
			return
		case <-ticker.C:
			if pair, err := m.service.MonitorPair(ctx, m.pairAddress); err == nil {
				if price, err := strconv.ParseFloat(pair.PriceNative, 64); err == nil {
					priceChange := ((price - m.initialPrice) / m.initialPrice) * 100

					update := PriceUpdate{
						PriceInSol:      price,
						PriceChangePerc: priceChange,
						LiquidityInSol:  pair.Liquidity.Quote,
						UpdateTime:      time.Now(),
					}

					select {
					case m.priceChangeChan <- update:
					default:
					}
				}
			}
		}
	}
}

func (m *PriceMonitor) Stop() {
	close(m.stopChan)
}

func (m *PriceMonitor) GetPriceUpdates() <-chan PriceUpdate {
	return m.priceChangeChan
}
