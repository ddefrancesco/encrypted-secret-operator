package crypto

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	hello "github.com/copsds/encrypted-secret-operator/internal/crypto/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	securityv1alpha1 "github.com/copsds/encrypted-secret-operator/api/v1alpha1"
)

type EncryptRequest struct {
	Data map[string]string `json:"data"`
}

type EncryptResponse struct {
	Ciphertext map[string]string `json:"ciphertext"`
}

type GenerateRequest struct {
	Type       string            `json:"type"`
	Parameters map[string]string `json:"parameters"`
}

type GenerateResponse struct {
	Data     map[string]string `json:"data"`
	Metadata struct {
		Version     string `json:"version"`
		GeneratedAt string `json:"generatedAt"`
		TTLSeconds  int    `json:"ttlSeconds"`
		ID          string `json:"id"`
	} `json:"metadata"`
}

// Encrypt encrypts data using the specified endpoint (HTTP or gRPC)
func Encrypt(endpoint securityv1alpha1.Endpoint, data map[string]string) (map[string]string, error) {
	switch strings.ToLower(endpoint.Protocol) {
	case "grpc":
		return encryptGRPC(endpoint, data)
	case "http", "":
		return encryptHTTP(endpoint, data)
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", endpoint.Protocol)
	}
}

// Generate generates data using the specified endpoint (HTTP or gRPC)
func Generate(endpoint securityv1alpha1.Endpoint, typ string, params map[string]string, idempotencyKey string) (*GenerateResponse, error) {
	switch strings.ToLower(endpoint.Protocol) {
	case "grpc":
		return generateGRPC(endpoint, typ, params, idempotencyKey)
	case "http", "":
		return generateHTTP(endpoint, typ, params, idempotencyKey)
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", endpoint.Protocol)
	}
}

// encryptHTTP handles HTTP-based encryption
func encryptHTTP(endpoint securityv1alpha1.Endpoint, data map[string]string) (map[string]string, error) {
	reqBody := EncryptRequest{
		Data: data,
	}

	body, _ := json.Marshal(reqBody)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	if endpoint.Insecure != nil && *endpoint.Insecure {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	resp, err := client.Post(endpoint.Address, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: %s", resp.Status)
	}

	var encResp EncryptResponse
	err = json.NewDecoder(resp.Body).Decode(&encResp)
	if err != nil {
		return nil, err
	}

	return encResp.Ciphertext, nil
}

// generateHTTP handles HTTP-based generation
func generateHTTP(endpoint securityv1alpha1.Endpoint,
	typ string,
	params map[string]string,
	idempotencyKey string,
) (*GenerateResponse, error) {
	reqBody := GenerateRequest{
		Type:       typ,
		Parameters: params,
	}

	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", endpoint.Address, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	if endpoint.Insecure != nil && *endpoint.Insecure {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: %s", resp.Status)
	}

	var genResp GenerateResponse
	err = json.NewDecoder(resp.Body).Decode(&genResp)
	if err != nil {
		return nil, err
	}

	return &genResp, nil
}

// encryptGRPC handles gRPC-based encryption
func encryptGRPC(endpoint securityv1alpha1.Endpoint, data map[string]string) (map[string]string, error) {
	opts := []grpc.DialOption{}

	if endpoint.Insecure != nil && *endpoint.Insecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(endpoint.Address, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server: %w", err)
	}
	defer conn.Close()

	client := hello.NewHelloGrpcClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ciphertext, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	req := &hello.EncryptRequest{
		Plaintext: string(ciphertext),
	}

	resp, err := client.Encrypt(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gRPC Encrypt call failed: %w", err)
	}

	return map[string]string{
		"ciphertext": resp.Ciphertext,
		"key_id":     resp.KeyId,
		"status":     resp.Status,
	}, nil
}

// generateGRPC handles gRPC-based generation
func generateGRPC(endpoint securityv1alpha1.Endpoint, typ string, params map[string]string, idempotencyKey string) (*GenerateResponse, error) {
	opts := []grpc.DialOption{}

	if endpoint.Insecure != nil && *endpoint.Insecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(endpoint.Address, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server: %w", err)
	}
	defer conn.Close()

	client := hello.NewHelloGrpcClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := &hello.GenerateRequest{
		Type:           typ,
		Parameters:     params,
		IdempotencyKey: idempotencyKey,
	}

	resp, err := client.Generate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gRPC Generate call failed: %w", err)
	}

	genResp := &GenerateResponse{
		Data: resp.Data,
	}
	genResp.Metadata.Version = resp.Metadata.Version
	genResp.Metadata.GeneratedAt = resp.Metadata.GeneratedAt
	genResp.Metadata.TTLSeconds = int(resp.Metadata.TtlSeconds)
	genResp.Metadata.ID = resp.Metadata.Id

	return genResp, nil
}

// Backward compatibility functions
// func Encrypt(endpoint string, data map[string]string) (map[string]string, error) {
// 	return encryptHTTP(securityv1alpha1.Endpoint{Protocol: "http", Address: endpoint}, data)
// }

// func Generate(endpoint string, typ string, params map[string]string) (*GenerateResponse, error) {
// 	return generateHTTP(securityv1alpha1.Endpoint{Protocol: "http", Address: endpoint}, typ, params)
// }
