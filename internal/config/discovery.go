package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func LoadResolved(ctx context.Context, path string) (*Config, string, error) {
	resolvedPath, err := ResolvePath(ctx, path)
	if err != nil {
		return nil, resolvedPath, err
	}

	cfg, err := Load(ctx, resolvedPath)
	if err != nil {
		return nil, resolvedPath, fmt.Errorf("load config: %w", err)
	}

	return cfg, resolvedPath, nil
}

func ResolvePath(ctx context.Context, path string) (string, error) {
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

	configDir, found, err := findAncestorContaining(ctx, workingDir, DefaultFile, searchRoot)
	if err != nil {
		return "", fmt.Errorf("discover config path: %w", err)
	}

	if !found {
		return DefaultFile, missingPathError(DefaultFile)
	}

	return filepath.Join(configDir, DefaultFile), nil
}

func ResolveInitPath(ctx context.Context, path string) (string, error) {
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

	configDir, found, err := findAncestorContaining(ctx, workingDir, DefaultFile, searchRoot)
	if err != nil {
		return "", fmt.Errorf("discover config path: %w", err)
	}

	if found {
		return filepath.Join(configDir, DefaultFile), nil
	}

	if searchRoot == "" {
		return DefaultFile, nil
	}

	return filepath.Join(searchRoot, DefaultFile), nil
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
