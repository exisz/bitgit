package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadToken reads a bearer token for the given host.
//
// Resolution order:
//  1. Environment variable (GITHUB_TOKEN for github, BITBUCKET_TOKEN for bitbucket-dc)
//  2. token_file path from [[hosts]] entry
//  3. Default secrets file: <config-dir>/secrets/<type>.token
//
// The file must be mode 0600; ReadToken returns an error if it is world-readable.
func (c *Config) ReadToken(hostType string) (string, error) {
	// 1. Env var
	switch strings.ToLower(hostType) {
	case "github":
		if t := os.Getenv("GITHUB_TOKEN"); t != "" {
			return t, nil
		}
	case "bitbucket-dc", "bitbucket_dc":
		if t := os.Getenv("BITBUCKET_TOKEN"); t != "" {
			return t, nil
		}
	}

	// 2. token_file from hosts config
	for _, h := range c.Hosts {
		if strings.EqualFold(h.Type, hostType) && h.TokenFile != "" {
			path := expandTilde(h.TokenFile, c.dir)
			tok, err := readSecureFile(path)
			if err != nil {
				return "", err
			}
			return tok, nil
		}
	}

	// 3. Default path
	name := strings.ToLower(strings.ReplaceAll(hostType, "-", "_"))
	defaultPath := filepath.Join(c.dir, "secrets", name+".token")
	if _, err := os.Stat(defaultPath); err == nil {
		return readSecureFile(defaultPath)
	}

	return "", nil // no token — caller decides if this is fatal
}

func readSecureFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("token file %s: %w", path, err)
	}
	if info.Mode().Perm()&0o044 != 0 {
		return "", fmt.Errorf("token file %s is group/world readable (mode %o); chmod 600", path, info.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read token file %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func expandTilde(path, dir string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(dir, path)
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
