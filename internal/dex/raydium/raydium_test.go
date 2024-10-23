// internal/dex/raydium/raydium_test.go
package raydium

import (
	"context"
	"errors"
	"testing"
	"time"

	solanaGo "github.com/gagliardetto/solana-go"
	"github.com/rovshanmuradov/solana-bot/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const defaultTestTimeout = 5 * time.Second

var TestPoolConfig = &Pool{
	AmmProgramID:               "675kPX9MHTjS2zt1qfr1NYHuzeLXfQM9H24wFSUt1Mp8",
	AmmID:                      "58oQChx4yWmvKdwLLZzBi4ChoCc2fqCUWBkwMihLYQo2",
	AmmAuthority:               "5Q544fKrFoe6tsEbD7S8EmxGTJYAKtTVhAW5Q5pge4j1",
	AmmOpenOrders:              "HRk9CMrpq7Qr6mhLkWGJR19dZ1P7RtUcYP3qHWKKYXAh",
	AmmTargetOrders:            "CZza3Ej4Mc58MnxWA385itCC9jCo3L1D7zc3LKy1bZMR",
	PoolCoinTokenAccount:       "DQyrAcCrDXQ7NeoqGgDCZwBvWDcYmFCjSb9JtteuvPpz",
	PoolPcTokenAccount:         "HLmqeL62xR1QoZ1HKKbXRrdN1p3phKpxRMb2VVopvBBz",
	SerumProgramID:             "9xQeWvG816bUx9EPjHmaT23yvVM2ZWbrrpZb9PusVFin",
	SerumMarket:                "DZjbn4XC8qoHKikZqzmhemykVzmossoayV9ffbsUqxVj",
	SerumBids:                  "DfiHxtHBUHwEXPEHAuruucxxRc8qYPZniGqBeqkMqZQF",
	SerumAsks:                  "BBLkJKGGxkxZsFWUMQB4HnLkADmMr3NFYN4YbC4LpmKJ",
	SerumEventQueue:            "8w4n3fcajhgN8TF74j42ehWvbVJnck5cewpjwhRQpyyc",
	SerumCoinVaultAccount:      "JCC1k8CrZX8QPAgwqErNWVg9m4ckVGADqHhYEDns8qJy",
	SerumPcVaultAccount:        "D8Lg4ASqHHpwBTZYfbm1v3wPqBUPvGP5ezm6gCaASTR6",
	SerumVaultSigner:           "Gdq3kzF8CGxzHVFHyVXAKxZdGj5MFokKvjzxCQ2WBZyd",
	RaydiumSwapInstructionCode: 9,
}

func TestNewDEX(t *testing.T) {
	mockClient := new(MockSolanaClient)
	dex := NewTestDEX(t, mockClient)

	assert.NotNil(t, dex)
	assert.Equal(t, "Raydium", dex.Name())
	assert.Equal(t, mockClient, dex.client)
	assert.Equal(t, TestPoolConfig, dex.poolInfo)
}

// Обновленный TestPrepareSwapInstruction без ожидания вызова GetRecentBlockhash
func TestPrepareSwapInstruction(t *testing.T) {
	tests := []struct {
		name          string
		amountIn      uint64
		minAmountOut  uint64
		expectedError bool
		errorMsg      string
	}{
		{
			name:          "Successful instruction preparation",
			amountIn:      1000000000,
			minAmountOut:  900000000,
			expectedError: false,
		},
		{
			name:          "Zero amount in",
			amountIn:      0,
			minAmountOut:  100000000,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(MockSolanaClient)
			dex := NewTestDEX(t, mockClient)

			ctx, cancel := MockedContext()
			defer cancel()

			wallet := solanaGo.NewWallet()
			sourceToken := solanaGo.NewWallet().PublicKey()
			destToken := solanaGo.NewWallet().PublicKey()

			instruction, err := dex.PrepareSwapInstruction(
				ctx,
				wallet.PublicKey(),
				sourceToken,
				destToken,
				tt.amountIn,
				tt.minAmountOut,
				dex.logger,
			)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, instruction)
			}
		})
	}
}

func TestPrepareAndSendTransaction(t *testing.T) {
	tests := []struct {
		name          string
		mockSetup     func(*MockSolanaClient)
		expectedError bool
		errorMsg      string
	}{
		{
			name: "Successful transaction preparation and sending",
			mockSetup: func(m *MockSolanaClient) {
				m.On("GetRecentBlockhash", mock.Anything).Return(solanaGo.Hash{}, nil)
				m.On("SendTransaction", mock.Anything, mock.Anything).Return(solanaGo.Signature{}, nil)
			},
			expectedError: false,
		},
		{
			name: "Failed to get blockhash",
			mockSetup: func(m *MockSolanaClient) {
				m.On("GetRecentBlockhash", mock.Anything).Return(solanaGo.Hash{}, errors.New("blockhash error"))
			},
			expectedError: true,
			errorMsg:      "blockhash error",
		},
		{
			name: "Failed to send transaction",
			mockSetup: func(m *MockSolanaClient) {
				m.On("GetRecentBlockhash", mock.Anything).Return(solanaGo.Hash{}, nil)
				m.On("SendTransaction", mock.Anything, mock.Anything).Return(solanaGo.Signature{}, errors.New("transaction error"))
			},
			expectedError: true,
			errorMsg:      "transaction error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(MockSolanaClient)
			tt.mockSetup(mockClient)
			dex := NewTestDEX(t, mockClient)

			testWallet := MockedWallet()
			swapInstruction := solanaGo.NewInstruction(
				solanaGo.SystemProgramID,
				nil,
				nil,
			)

			err := dex.PrepareAndSendTransaction(context.Background(), &types.Task{}, testWallet, dex.logger, swapInstruction)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestExecuteSwap(t *testing.T) {
	tests := []struct {
		name          string
		task          *types.Task
		mockSetup     func(*MockSolanaClient)
		expectedError bool
		errorMsg      string
	}{
		{
			name: "Successful swap execution",
			task: &types.Task{
				AmountIn:                    1.0,
				MinAmountOut:                0.9,
				SourceTokenDecimals:         9,
				TargetTokenDecimals:         9,
				UserSourceTokenAccount:      solanaGo.NewWallet().PublicKey(),
				UserDestinationTokenAccount: solanaGo.NewWallet().PublicKey(),
				PriorityFee:                 0.000001,
			},
			mockSetup: func(m *MockSolanaClient) {
				m.On("GetRecentBlockhash", mock.Anything).Return(solanaGo.Hash{}, nil)
				m.On("SendTransaction", mock.Anything, mock.Anything).Return(solanaGo.Signature{}, nil)
			},
			expectedError: false,
		},
		{
			name: "Failed to get blockhash",
			task: &types.Task{
				AmountIn:                    1.0,
				MinAmountOut:                0.9,
				SourceTokenDecimals:         9,
				TargetTokenDecimals:         9,
				UserSourceTokenAccount:      solanaGo.NewWallet().PublicKey(),
				UserDestinationTokenAccount: solanaGo.NewWallet().PublicKey(),
			},
			mockSetup: func(m *MockSolanaClient) {
				m.On("GetRecentBlockhash", mock.Anything).Return(solanaGo.Hash{}, errors.New("blockhash error"))
			},
			expectedError: true,
			errorMsg:      "blockhash error",
		},
		{
			name: "Failed to send transaction",
			task: &types.Task{
				AmountIn:                    1.0,
				MinAmountOut:                0.9,
				SourceTokenDecimals:         9,
				TargetTokenDecimals:         9,
				UserSourceTokenAccount:      solanaGo.NewWallet().PublicKey(),
				UserDestinationTokenAccount: solanaGo.NewWallet().PublicKey(),
			},
			mockSetup: func(m *MockSolanaClient) {
				m.On("GetRecentBlockhash", mock.Anything).Return(solanaGo.Hash{}, nil)
				m.On("SendTransaction", mock.Anything, mock.Anything).Return(solanaGo.Signature{}, errors.New("transaction error"))
			},
			expectedError: true,
			errorMsg:      "transaction error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(MockSolanaClient)
			tt.mockSetup(mockClient)
			dex := NewTestDEX(t, mockClient)

			testWallet := MockedWallet()

			err := dex.ExecuteSwap(context.Background(), tt.task, testWallet)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestCreateSwapInstruction(t *testing.T) {
	tests := []struct {
		name          string
		amountIn      uint64
		minAmountOut  uint64
		expectedError bool
		checkResult   func(*testing.T, solanaGo.Instruction)
	}{
		{
			name:          "Valid instruction creation",
			amountIn:      1000000000,
			minAmountOut:  900000000,
			expectedError: false,
			checkResult: func(t *testing.T, instruction solanaGo.Instruction) {
				// Вызов метода Accounts()
				assert.Len(t, instruction.Accounts(), 20)

				// Вызов метода ProgramID()
				expectedProgramID := solanaGo.MustPublicKeyFromBase58(TestPoolConfig.AmmProgramID)
				assert.Equal(t, expectedProgramID, instruction.ProgramID())

				// Вызов метода Data()
				data, err := instruction.Data()
				assert.NoError(t, err)
				assert.NotEmpty(t, data)
			},
		},
		{
			name:          "Zero amounts",
			amountIn:      0,
			minAmountOut:  0,
			expectedError: false,
			checkResult: func(t *testing.T, instruction solanaGo.Instruction) {
				// Вызов метода Accounts()
				assert.Len(t, instruction.Accounts(), 20)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(MockSolanaClient)
			dex := NewTestDEX(t, mockClient)

			wallet := solanaGo.NewWallet()
			sourceToken := solanaGo.NewWallet().PublicKey()
			destToken := solanaGo.NewWallet().PublicKey()

			instruction, err := dex.CreateSwapInstruction(
				wallet.PublicKey(),
				sourceToken,
				destToken,
				tt.amountIn,
				tt.minAmountOut,
				dex.logger,
				TestPoolConfig,
			)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, instruction)
				tt.checkResult(t, instruction)
			}
		})
	}
}

func TestSwapInstructionDataSerialization(t *testing.T) {
	tests := []struct {
		name        string
		data        SwapInstructionData
		wantSize    int
		checkResult func(*testing.T, []byte)
	}{
		{
			name: "Valid data serialization",
			data: SwapInstructionData{
				Instruction:  9,
				AmountIn:     1000000000,
				MinAmountOut: 900000000,
			},
			wantSize: 24, // 8 + 8 + 8 bytes for each uint64
			checkResult: func(t *testing.T, data []byte) {
				assert.Len(t, data, 24)
				// Можно добавить проверку конкретных байтов, если это необходимо
			},
		},
		{
			name: "Zero values serialization",
			data: SwapInstructionData{
				Instruction:  0,
				AmountIn:     0,
				MinAmountOut: 0,
			},
			wantSize: 24,
			checkResult: func(t *testing.T, data []byte) {
				assert.Len(t, data, 24)
				// Проверяем, что все байты нулевые
				for _, b := range data {
					assert.Equal(t, byte(0), b)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serialized, err := tt.data.Serialize()
			assert.NoError(t, err)
			assert.Equal(t, tt.wantSize, len(serialized))
			tt.checkResult(t, serialized)
		})
	}
}
