package tnstorage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrVaultNotFound = errors.New("vault not found")

// FindVault finds the vault by traversing up the filepath.
func FindVault(wd string) (string, error) {
	stat, err := os.Stat(wd)
	if err != nil {
		return "", fmt.Errorf("stat directory: %w", err)
	}

	vaultDir := filepath.Join(wd, ".obsidian")

	vaultStat, err := os.Stat(vaultDir)
	if err != nil {
		return "", fmt.Errorf("stat vault directory(%s): %w", vaultDir, err)
	}

	if !vaultStat.IsDir() {
		return "", fmt.Errorf("vault directory(%s) is not a directory", vaultDir)
	}

	_ = stat
	_ = vaultStat

	absVaultDir, err := filepath.Abs(vaultDir)
	if err != nil {
		return "", fmt.Errorf("abs vault directory(%s): %w", vaultDir, err)
	}

	return absVaultDir, nil
}
