package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/monkescience/yeet/internal/config"
)

func loadConfig(ctx context.Context, path string) (*config.Config, string, error) {
	resolvedPath, err := resolveConfigPath(ctx, path)
	if err != nil {
		return nil, resolvedPath, err
	}

	cfg, err := config.Load(ctx, resolvedPath)
	if err != nil {
		return nil, resolvedPath, fmt.Errorf("load config: %w", err)
	}

	return cfg, resolvedPath, nil
}

func resolveConfigPath(ctx context.Context, path string) (string, error) {
	explicitPath, hasExplicitPath := explicitConfigPath(path)
	if hasExplicitPath {
		return explicitPath, nil
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	searchRoot, err := configSearchRoot(ctx, workingDir)
	if err != nil {
		return "", err
	}

	configDir, found, err := findAncestorContaining(ctx, workingDir, config.DefaultFile, searchRoot)
	if err != nil {
		return "", fmt.Errorf("discover config path: %w", err)
	}

	if !found {
		return config.DefaultFile, missingPathError(config.DefaultFile)
	}

	return filepath.Join(configDir, config.DefaultFile), nil
}

func resolveInitConfigPath(ctx context.Context, path string) (string, error) {
	explicitPath, hasExplicitPath := explicitConfigPath(path)
	if hasExplicitPath {
		return explicitPath, nil
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	searchRoot, err := configSearchRoot(ctx, workingDir)
	if err != nil {
		return "", err
	}

	configDir, found, err := findAncestorContaining(ctx, workingDir, config.DefaultFile, searchRoot)
	if err != nil {
		return "", fmt.Errorf("discover config path: %w", err)
	}

	if found {
		return filepath.Join(configDir, config.DefaultFile), nil
	}

	if searchRoot == "" {
		return config.DefaultFile, nil
	}

	return filepath.Join(searchRoot, config.DefaultFile), nil
}

func explicitConfigPath(path string) (string, bool) {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return "", false
	}

	return trimmedPath, true
}

func configSearchRoot(ctx context.Context, startDir string) (string, error) {
	repositoryRoot, found, err := findAncestorContaining(ctx, startDir, ".git", "")
	if err != nil {
		return "", fmt.Errorf("discover git repository: %w", err)
	}

	if !found {
		return "", nil
	}

	return repositoryRoot, nil
}

func findAncestorContaining(
	ctx context.Context,
	startDir string,
	targetName string,
	stopDir string,
) (string, bool, error) {
	currentDir, err := filepath.Abs(startDir)
	if err != nil {
		return "", false, fmt.Errorf("resolve absolute path for %s: %w", startDir, err)
	}

	resolvedStopDir := ""
	if stopDir != "" {
		resolvedStopDir, err = filepath.Abs(stopDir)
		if err != nil {
			return "", false, fmt.Errorf("resolve absolute stop path for %s: %w", stopDir, err)
		}
	}

	for {
		ctxErr := ctx.Err()
		if ctxErr != nil {
			return "", false, fmt.Errorf("ancestor search cancelled: %w", ctxErr)
		}

		candidatePath := filepath.Join(currentDir, targetName)

		_, err := os.Stat(candidatePath)
		switch {
		case err == nil:
			return currentDir, true, nil
		case errors.Is(err, os.ErrNotExist):
		default:
			return "", false, fmt.Errorf("stat %s: %w", candidatePath, err)
		}

		if resolvedStopDir != "" && currentDir == resolvedStopDir {
			return "", false, nil
		}

		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			return "", false, nil
		}

		currentDir = parentDir
	}
}

func missingPathError(path string) error {
	return &os.PathError{Op: "stat", Path: path, Err: os.ErrNotExist}
}
