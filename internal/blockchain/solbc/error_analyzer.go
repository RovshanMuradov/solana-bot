package solbc

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gagliardetto/solana-go/rpc/jsonrpc"
	"go.uber.org/zap"
)

// AnchorError represents an error from Anchor framework
type AnchorError struct {
	Code        int    `json:"code"`
	Name        string `json:"name"`
	Msg         string `json:"msg"`
	ProgramID   string `json:"programId,omitempty"`
	Instruction int    `json:"instruction,omitempty"`
}

// ErrorAnalyzer provides methods to analyze Solana transaction errors
type ErrorAnalyzer struct {
	logger *zap.Logger
}

// NewErrorAnalyzer creates a new ErrorAnalyzer instance
func NewErrorAnalyzer(logger *zap.Logger) *ErrorAnalyzer {
	return &ErrorAnalyzer{
		logger: logger.Named("error-analyzer"),
	}
}

// AnalyzeRPCError analyzes a jsonrpc.RPCError and extracts detailed information
func (ea *ErrorAnalyzer) AnalyzeRPCError(err error) map[string]interface{} {
	if err == nil {
		return map[string]interface{}{
			"error": "No error provided",
		}
	}

	// Check if it's an RPC error
	rpcErr, ok := err.(*jsonrpc.RPCError)
	if !ok {
		return map[string]interface{}{
			"type":    "generic_error",
			"message": err.Error(),
		}
	}

	// Extract error data
	result := map[string]interface{}{
		"type":    "rpc_error",
		"code":    rpcErr.Code,
		"message": rpcErr.Message,
	}

	// Check if this is a transaction simulation error
	if strings.Contains(rpcErr.Message, "Transaction simulation failed") {
		result["simulation_failed"] = true

		// Extract simulation details from data
		if rpcErr.Data != nil {
			if dataMap, ok := rpcErr.Data.(map[string]interface{}); ok {
				// Extract logs
				if logs, ok := dataMap["logs"].([]interface{}); ok {
					result["logs"] = logs

					// Look for Anchor error in logs
					for _, logEntry := range logs {
						if logStr, ok := logEntry.(string); ok {
							if strings.Contains(logStr, "AnchorError occurred") {
								anchorErr := ea.parseAnchorErrorLog(logStr)
								result["anchor_error"] = anchorErr

								// Log detailed analysis
								ea.logger.Warn("Anchor error detected",
									zap.Int("code", anchorErr.Code),
									zap.String("name", anchorErr.Name),
									zap.String("message", anchorErr.Msg))
							}
						}
					}
				}

				// Extract instruction error
				if err, ok := dataMap["err"].(map[string]interface{}); ok {
					result["instruction_error"] = err
				}
			}
		}
	}

	return result
}

// parseAnchorErrorLog parses an Anchor error log string
// Example: "Program log: AnchorError occurred. Error Code: InstructionFallbackNotFound. Error Number: 101. Error Message: Fallback functions are not supported."
func (ea *ErrorAnalyzer) parseAnchorErrorLog(logStr string) AnchorError {
	result := AnchorError{}

	// Extract error code
	if strings.Contains(logStr, "Error Number:") {
		parts := strings.Split(logStr, "Error Number:")
		if len(parts) > 1 {
			numParts := strings.Split(parts[1], ".")
			if len(numParts) > 0 {
				fmt.Sscanf(strings.TrimSpace(numParts[0]), "%d", &result.Code)
			}
		}
	}

	// Extract error name
	if strings.Contains(logStr, "Error Code:") {
		parts := strings.Split(logStr, "Error Code:")
		if len(parts) > 1 {
			nameParts := strings.Split(parts[1], ".")
			if len(nameParts) > 0 {
				result.Name = strings.TrimSpace(nameParts[0])
			}
		}
	}

	// Extract error message
	if strings.Contains(logStr, "Error Message:") {
		parts := strings.Split(logStr, "Error Message:")
		if len(parts) > 1 {
			result.Msg = strings.TrimSpace(strings.Split(parts[1], ".")[0])
		}
	}

	return result
}

// FormatErrorAnalysis formats the error analysis for logging or display
func (ea *ErrorAnalyzer) FormatErrorAnalysis(analysis map[string]interface{}) string {
	jsonBytes, err := json.MarshalIndent(analysis, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error formatting analysis: %v", err)
	}
	return string(jsonBytes)
}
