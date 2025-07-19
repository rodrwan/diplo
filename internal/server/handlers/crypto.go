package handlers

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
)

// getEncryptionKey obtiene la clave de cifrado desde variables de entorno
// En producción, esto debería ser gestionado de forma más segura
func getEncryptionKey() ([]byte, error) {
	key := os.Getenv("DIPLO_ENCRYPTION_KEY")
	if key == "" {
		// Generar una clave por defecto para desarrollo
		// En producción, esto debería fallar si no se proporciona una clave
		key = "diplo-default-key-32-chars-long"
	}

	if len(key) != 32 {
		return nil, errors.New("la clave de cifrado debe tener exactamente 32 caracteres")
	}

	return []byte(key), nil
}

// encryptValue cifra un valor usando AES-GCM
func encryptValue(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	key, err := getEncryptionKey()
	if err != nil {
		return "", fmt.Errorf("error obteniendo clave de cifrado: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("error creando cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("error creando GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("error generando nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptValue descifra un valor usando AES-GCM
func decryptValue(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	key, err := getEncryptionKey()
	if err != nil {
		return "", fmt.Errorf("error obteniendo clave de cifrado: %w", err)
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("error decodificando base64: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("error creando cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("error creando GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("datos de cifrado inválidos")
	}

	nonce, cipherData := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return "", fmt.Errorf("error descifrando: %w", err)
	}

	return string(plaintext), nil
}

// DecryptValue descifra un valor usando AES-GCM (función pública)
func DecryptValue(ciphertext string) (string, error) {
	return decryptValue(ciphertext)
}

// shouldEncryptValue determina si un valor debe ser cifrado
func shouldEncryptValue(isSecret bool) bool {
	return isSecret
}
