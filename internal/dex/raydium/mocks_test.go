// internal/dex/raydium/mocks_test.go
package raydium

import (
	"context"
	"testing"

	solanaGo "github.com/gagliardetto/solana-go"
	internalSolana "github.com/rovshanmuradov/solana-bot/internal/blockchain/solana"
	"github.com/rovshanmuradov/solana-bot/internal/wallet"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

// MockSolanaClient реализует интерфейс internalSolana.SolanaClientInterface
type MockSolanaClient struct {
	mock.Mock
}

func (m *MockSolanaClient) GetRecentBlockhash(ctx context.Context) (solanaGo.Hash, error) {
	args := m.Called(ctx)
	return args.Get(0).(solanaGo.Hash), args.Error(1)
}

func (m *MockSolanaClient) SendTransaction(ctx context.Context, tx *solanaGo.Transaction) (solanaGo.Signature, error) {
	args := m.Called(ctx, tx)
	return args.Get(0).(solanaGo.Signature), args.Error(1)
}

// NewTestDEX создает DEX с моком клиента для тестирования
func NewTestDEX(t *testing.T, mockClient internalSolana.SolanaClientInterface) *DEX {
	logger := zap.NewNop()

	if mockClient == nil {
		mc := new(MockSolanaClient)
		mc.On("GetRecentBlockhash", mock.Anything).Return(
			solanaGo.Hash{},
			nil,
		)
		mc.On("SendTransaction", mock.Anything, mock.Anything).Return(
			solanaGo.Signature{},
			nil,
		)
		mockClient = mc
	}

	return &DEX{
		client:   mockClient,
		logger:   logger,
		poolInfo: TestPoolConfig,
	}
}

// MockedContext создает контекст с таймаутом для тестов
func MockedContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), defaultTestTimeout)
}

// MockedWallet создает тестовый кошелек
func MockedWallet() *wallet.Wallet {
	w := solanaGo.NewWallet()
	return &wallet.Wallet{
		PrivateKey: w.PrivateKey,
		PublicKey:  w.PublicKey(),
	}
}
