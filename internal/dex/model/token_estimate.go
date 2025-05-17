// internal/dex/model/token_estimate.go
package model

// TokenEstimate представляет оценку стоимости токенов в SOL
type TokenEstimate struct {
	// TokenMint адрес минта токена
	TokenMint string

	// TokenBalance баланс токена в натуральных единицах
	TokenBalance uint64

	// TokenPrice цена токена в SOL
	TokenPrice float64

	// EstimatedValue оценочная стоимость в лампортах
	EstimatedValue uint64

	// HumanBalance баланс токена в человекочитаемом формате
	HumanBalance float64

	// HumanEstimated оценочная стоимость в SOL
	HumanEstimated float64

	// TokenPrecision точность токена
	TokenPrecision uint8
}
