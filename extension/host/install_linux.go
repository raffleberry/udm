//go:build linux

package host

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func Install() error {
	err := checkBrowserInstalled()
	if err != nil {
		slog.Error("Failed while checking for browser", "err", err)
		return err
	}

	err = installHost()
	if err != nil {
		slog.Error("Failed to install host", "err", err)
		return err
	}

	return nil
}

func checkBrowserInstalled() error {
	_, err := exec.LookPath("firefox")
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("firefox is not installed: %w", err)
	}
	return nil
}

func getPkgDir() (string, error) {
	hostDir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	pkgDir := filepath.Join(hostDir, "extfiles")
	err = os.MkdirAll(pkgDir, 0755)
	if err != nil {
		return "", err
	}

	hostBin := filepath.Join(pkgDir, "udm-browser-integration-host")
	if _, err := os.Stat(hostBin); err != nil {
		if !IsGoRun() {
			return "", fmt.Errorf("host file binary[%s] not found : %w", hostBin, err)
		}
		slog.Warn("extfiles directory not found in pkgDir", "path", hostBin)
		slog.Info("compiling host", "path", hostBin)

		err := exec.Command("go", "build", "-o", hostBin, filepath.Join("cmd", "host", "host.go")).Run()
		if err != nil {
			return "", fmt.Errorf("Failed to compile host binary: %w", err)
		}
		slog.Info("compile success")
	}

	return pkgDir, nil
}

func installHost() error {
	pkgDir, err := getPkgDir()
	if err != nil {
		return fmt.Errorf("error while preparing/getting the packaged files dir: %w", err)
	}

	//default dirs
	homeDir, err := os.UserHomeDir()
	if err != nil {
		slog.Error("error getting home dir", "err", err)
		return err
	}
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		slog.Error("error getting config dir", "err", err)
		return err
	}

	//copy and reg the host binary
	udmCfgDir := filepath.Join(cfgDir, "udm_raffleberry")
	err = os.MkdirAll(udmCfgDir, 0755)
	if err != nil {
		slog.Error("error creating udm config dir", "err", err)
		return err
	}
	extHost := "udm-browser-integration-host"
	pkgHost := filepath.Join(pkgDir, extHost)
	installHost := filepath.Join(udmCfgDir, extHost)
	slog.Info("copying host", "src", pkgHost, "dst", installHost)
	err = cp(pkgHost, installHost)
	if err != nil {
		slog.Error("error copying host", "err", err)
		return err
	}
	err = os.Chmod(installHost, 0744)
	if err != nil {
		slog.Error("error changing host permissions", "err", err)
		return err
	}
	slog.Info("Copied host", "bin", installHost)

	//copy and register the json
	extRegDir := filepath.Join(homeDir, ".mozilla", "native-messaging-hosts")
	err = os.MkdirAll(extRegDir, 0755)
	if err != nil {
		slog.Error("error creating firefox host dir", "err", err)
		return err
	}
	extJson := "raffleberry.udm.json"
	pkgJson := filepath.Join(pkgDir, extJson)
	installJson := filepath.Join(extRegDir, extJson)
	slog.Info("copying host json", "src", pkgJson, "dst", installJson)
	err = cp(pkgJson, installJson)
	if err != nil {
		slog.Error("error copying host json", "err", err)
		return err
	}
	slog.Info("Copied host json", "json", pkgJson)
	content, err := os.ReadFile(installJson)
	if err != nil {
		slog.Error("error reading host json", "err", err)
		return err
	}
	newContent := strings.ReplaceAll(string(content), "__NATIVE_BIN_INSTALL_PATH__", installHost)
	err = os.WriteFile(installJson, []byte(newContent), 0644)
	if err != nil {
		slog.Error("error writing host json", "err", err)
		return err
	}
	slog.Info("Updated host json", "json", installJson)

	return nil
}

func cp(src, dst string) error {
	// 1. Open the source file for reading
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}

	err = destFile.Sync()
	if err != nil {
		return fmt.Errorf("failed to sync destination file: %w", err)
	}

	return nil
}
