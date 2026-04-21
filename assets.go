package main

import (
	"os"
	"fmt"
	"path/filepath"
	"strings"
	"encoding/base64"
	"crypto/rand"

)

func (cfg apiConfig) ensureAssetsDir() error {
	if _, err := os.Stat(cfg.assetsRoot); os.IsNotExist(err) {
		return os.Mkdir(cfg.assetsRoot, 0755)
	}
	return nil
}

func getAssetPath(mediaType string) string {
	assetKey := make([]byte, 32)
	_, err := rand.Read(assetKey)
	if err != nil {
		panic("failed to generate random bytes")
	}
	assetID := base64.RawURLEncoding.EncodeToString(assetKey)

	ext := mediaTypeToExt(mediaType)
	return fmt.Sprintf("%s%s", assetID, ext)
}

func (cfg apiConfig) getAssetDiskPath(assetPath string) string {
	return filepath.Join(cfg.assetsRoot, assetPath)
}

func (cfg apiConfig) getAssetURL(assetPath string) string {
	return fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, assetPath)
}

func mediaTypeToExt(mediaType string) string {
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 {
		return ".bin"
	}
	return "." + parts[1]
}

func (cfg apiConfig) getObjectURL(key string) string {
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, key)
}