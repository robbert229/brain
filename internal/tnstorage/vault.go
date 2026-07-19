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
	_, err := os.Stat(wd)
	if err != nil {
		return "", fmt.Errorf("stat directory: %w", err)
	}

	absWd, err := filepath.Abs(wd)
	if err != nil {
		return "", fmt.Errorf("get abs path: %w", err)
	}

	absIterWd := absWd
	for absIterWd != "/" {
		absIterVaultDir := filepath.Join(absIterWd, ".obsidian")
		vaultStat, err := os.Stat(absIterVaultDir)
		if err != nil {
			absIterWd = filepath.Dir(absIterWd)

			continue
		}

		if !vaultStat.IsDir() {
			return "", fmt.Errorf("vault directory(%s) is not a directory", absIterVaultDir)
		}

		return absIterVaultDir, nil
	}

	return "", ErrVaultNotFound
}
