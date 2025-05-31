// internal/license/keygen.go
package license

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"runtime"

	"github.com/keygen-sh/keygen-go/v3"
	"go.uber.org/zap"
)

// KeygenValidator handles license validation using Keygen.sh
type KeygenValidator struct {
	logger    *zap.Logger
	accountID string
	productID string
}

// NewKeygenValidator creates a new Keygen license validator
func NewKeygenValidator(accountID, productToken, productID string, logger *zap.Logger) *KeygenValidator {
	// Configure global Keygen settings
	keygen.Account = accountID
	keygen.Product = productID
	keygen.Token = productToken
	keygen.PublicKey = "" // Will be fetched automatically

	return &KeygenValidator{
		logger:    logger,
		accountID: accountID,
		productID: productID,
	}
}

// ValidateLicense validates a license key with Keygen
func (kv *KeygenValidator) ValidateLicense(ctx context.Context, licenseKey string) error {
	kv.logger.Info("🔑 Validating license: " + licenseKey[:8] + "...")

	// Generate machine fingerprint
	fingerprint, err := kv.generateFingerprint()
	if err != nil {
		return fmt.Errorf("failed to generate machine fingerprint: %w", err)
	}

	// Set the license key for validation
	keygen.LicenseKey = licenseKey

	// Validate license
	license, err := keygen.Validate(ctx, fingerprint)
	switch {
	case err == keygen.ErrLicenseNotActivated:
		kv.logger.Info("License not activated, attempting activation")
		// Try to activate the license
		machine, activateErr := license.Activate(ctx, fingerprint)
		if activateErr != nil {
			return fmt.Errorf("failed to activate license: %w", activateErr)
		}
		kv.logger.Info("License activated successfully",
			zap.String("machine_id", machine.ID),
			zap.String("fingerprint", fingerprint),
		)

	case err == keygen.ErrLicenseExpired:
		return fmt.Errorf("license has expired")

	case err != nil:
		return fmt.Errorf("license validation failed: %w", err)
	}

	if license == nil {
		return fmt.Errorf("license not found")
	}

	kv.logger.Info("License validation successful",
		zap.String("license_id", license.ID),
	)

	return nil
}

// generateFingerprint creates a unique machine fingerprint
func (kv *KeygenValidator) generateFingerprint() (string, error) {
	// Get MAC addresses
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	var macAddresses []string
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			macAddresses = append(macAddresses, iface.HardwareAddr.String())
		}
	}

	if len(macAddresses) == 0 {
		return "", fmt.Errorf("no network interfaces found")
	}

	// Get hostname
	hostname, err := getHostname()
	if err != nil {
		return "", err
	}

	// Create fingerprint from hostname + MAC addresses + OS
	data := fmt.Sprintf("%s-%s-%s", hostname, macAddresses[0], runtime.GOOS)
	hash := sha256.Sum256([]byte(data))

	return fmt.Sprintf("%x", hash), nil
}

// getHostname returns the machine hostname
func getHostname() (string, error) {
	hostname, err := net.LookupCNAME("localhost")
	if err != nil {
		// Fallback to simpler method
		interfaces, err := net.Interfaces()
		if err != nil {
			return "unknown", err
		}
		if len(interfaces) > 0 {
			return interfaces[0].Name, nil
		}
		return "unknown", nil
	}
	return hostname, nil
}

// HeartbeatLicense sends a heartbeat to keep the license active
func (kv *KeygenValidator) HeartbeatLicense(ctx context.Context, licenseKey string) error {
	// Set the license key
	keygen.LicenseKey = licenseKey

	// Generate machine fingerprint
	fingerprint, err := kv.generateFingerprint()
	if err != nil {
		return fmt.Errorf("failed to generate machine fingerprint: %w", err)
	}

	// Re-validate to send heartbeat
	_, err = keygen.Validate(ctx, fingerprint)
	if err != nil {
		return fmt.Errorf("heartbeat failed: %w", err)
	}

	kv.logger.Debug("License heartbeat sent successfully")
	return nil
}
