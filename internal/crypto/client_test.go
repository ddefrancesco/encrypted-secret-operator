package crypto

import (
	"context"
	"fmt"
	"net"
	"testing"

	securityv1alpha1 "github.com/copsds/encrypted-secret-operator/api/v1alpha1"
	hello "github.com/copsds/encrypted-secret-operator/internal/crypto/proto"
	"google.golang.org/grpc"
)

// MockHelloGrpcServer implementa il server gRPC per i test
type MockHelloGrpcServer struct {
	hello.UnimplementedHelloGrpcServer
}

func (s *MockHelloGrpcServer) SayHello(ctx context.Context, req *hello.HelloRequest) (*hello.HelloReply, error) {
	return &hello.HelloReply{Message: "Hello " + req.Name}, nil
}

func (s *MockHelloGrpcServer) Generate(ctx context.Context, req *hello.GenerateRequest) (*hello.GenerateResponse, error) {
	if req.Type == "" {
		return nil, fmt.Errorf("type is required")
	}

	return &hello.GenerateResponse{
		Data: map[string]string{
			"secret": "generated-value-123",
			"key":    "test-key",
		},
		Metadata: &hello.Metadata{
			Version:     "1.0",
			GeneratedAt: "2026-04-22T10:00:00Z",
			TtlSeconds:  3600,
			Id:          "gen-id-abc123",
		},
	}, nil
}

func (s *MockHelloGrpcServer) Encrypt(ctx context.Context, req *hello.EncryptRequest) (*hello.EncryptResponse, error) {
	if req.Plaintext == "" {
		return nil, fmt.Errorf("plaintext is required")
	}

	return &hello.EncryptResponse{
		KeyId:      "key-1",
		Ciphertext: "encrypted-" + req.Plaintext,
		Status:     "success",
	}, nil
}

func (s *MockHelloGrpcServer) Store(ctx context.Context, req *hello.StoreRequest) (*hello.StoreResponse, error) {
	return &hello.StoreResponse{
		SecretId:  req.SecretId,
		Status:    "stored",
		Timestamp: 1234567890,
	}, nil
}

// startMockGrpcServer avvia un server gRPC mockato su una porta disponibile
func startMockGrpcServer(t *testing.T) (string, func()) {
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	server := grpc.NewServer()
	hello.RegisterHelloGrpcServer(server, &MockHelloGrpcServer{})

	go func() {
		if err := server.Serve(lis); err != nil {
			t.Logf("server error: %v", err)
		}
	}()

	// Restituisci l'indirizzo e una funzione per fermare il server
	return lis.Addr().String(), func() {
		server.Stop()
		lis.Close()
	}
}

// TestGenerateGRPCSuccess testa la generazione con successo
func TestGenerateGRPCSuccess(t *testing.T) {
	addr, cleanup := startMockGrpcServer(t)
	defer cleanup()

	endpoint := securityv1alpha1.Endpoint{
		Protocol: "grpc",
		Address:  addr,
		Insecure: boolPtr(true),
	}

	params := map[string]string{
		"length": "32",
		"type":   "password",
	}

	resp, err := generateGRPC(endpoint, "password", params)
	if err != nil {
		t.Fatalf("generateGRPC failed: %v", err)
	}

	if resp == nil {
		t.Fatal("response is nil")
	}

	if resp.Data["secret"] != "generated-value-123" {
		t.Errorf("expected secret value, got: %v", resp.Data["secret"])
	}

	if resp.Metadata.Version != "1.0" {
		t.Errorf("expected version 1.0, got: %s", resp.Metadata.Version)
	}

	if resp.Metadata.ID != "gen-id-abc123" {
		t.Errorf("expected ID gen-id-abc123, got: %s", resp.Metadata.ID)
	}

	if resp.Metadata.TTLSeconds != 3600 {
		t.Errorf("expected TTL 3600, got: %d", resp.Metadata.TTLSeconds)
	}
}

// TestGenerateGRPCConnectionError testa errore di connessione
func TestGenerateGRPCConnectionError(t *testing.T) {
	endpoint := securityv1alpha1.Endpoint{
		Protocol: "grpc",
		Address:  "localhost:0", // Porta non disponibile
		Insecure: boolPtr(true),
	}

	_, err := generateGRPC(endpoint, "password", map[string]string{})
	if err == nil {
		t.Fatal("expected error but got none")
	}

	if err.Error() == "" {
		t.Fatal("error message is empty")
	}
}

// TestEncryptGRPCSuccess testa l'encryption con successo
func TestEncryptGRPCSuccess(t *testing.T) {
	addr, cleanup := startMockGrpcServer(t)
	defer cleanup()

	endpoint := securityv1alpha1.Endpoint{
		Protocol: "grpc",
		Address:  addr,
		Insecure: boolPtr(true),
	}

	data := map[string]string{
		"username": "admin",
		"password": "secret123",
	}

	resp, err := encryptGRPC(endpoint, data)
	if err != nil {
		t.Fatalf("encryptGRPC failed: %v", err)
	}

	if resp == nil {
		t.Fatal("response is nil")
	}

	if resp["status"] != "success" {
		t.Errorf("expected status success, got: %s", resp["status"])
	}

	if resp["key_id"] != "key-1" {
		t.Errorf("expected key_id key-1, got: %s", resp["key_id"])
	}

	if resp["ciphertext"] == "" {
		t.Fatal("ciphertext is empty")
	}
}

// TestEncryptGRPCConnectionError testa errore di connessione
func TestEncryptGRPCConnectionError(t *testing.T) {
	endpoint := securityv1alpha1.Endpoint{
		Protocol: "grpc",
		Address:  "localhost:0", // Porta non disponibile
		Insecure: boolPtr(true),
	}

	_, err := encryptGRPC(endpoint, map[string]string{})
	if err == nil {
		t.Fatal("expected error but got none")
	}

	if err.Error() == "" {
		t.Fatal("error message is empty")
	}
}

// TestEncryptHTTPSuccess testa HTTP encryption
func TestEncryptHTTPSuccess(t *testing.T) {
	// Questo test richiede un server HTTP disponibile
	// Per ora lo saltiamo in quanto il focus è su gRPC
	t.Skip("HTTP server mock not implemented")
}

// TestGenerateHTTPSuccess testa HTTP generation
func TestGenerateHTTPSuccess(t *testing.T) {
	// Questo test richiede un server HTTP disponibile
	// Per ora lo saltiamo in quanto il focus è su gRPC
	t.Skip("HTTP server mock not implemented")
}

// TestEncryptWithGRPCProtocol testa il dispatcher Encrypt con protocollo gRPC
func TestEncryptWithGRPCProtocol(t *testing.T) {
	addr, cleanup := startMockGrpcServer(t)
	defer cleanup()

	endpoint := securityv1alpha1.Endpoint{
		Protocol: "grpc",
		Address:  addr,
		Insecure: boolPtr(true),
	}

	data := map[string]string{"key": "value"}

	resp, err := Encrypt(endpoint, data)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if resp == nil {
		t.Fatal("response is nil")
	}
}

// TestGenerateWithGRPCProtocol testa il dispatcher Generate con protocollo gRPC
func TestGenerateWithGRPCProtocol(t *testing.T) {
	addr, cleanup := startMockGrpcServer(t)
	defer cleanup()

	endpoint := securityv1alpha1.Endpoint{
		Protocol: "grpc",
		Address:  addr,
		Insecure: boolPtr(true),
	}

	resp, err := Generate(endpoint, "password", map[string]string{"length": "32"})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if resp == nil {
		t.Fatal("response is nil")
	}

	if len(resp.Data) == 0 {
		t.Fatal("response data is empty")
	}
}

// TestEncryptWithHTTPProtocol testa il dispatcher Encrypt con protocollo HTTP (fallback)
func TestEncryptWithHTTPProtocol(t *testing.T) {
	// Endpoint non valido, ma testa che il dispatcher routing funziona
	endpoint := securityv1alpha1.Endpoint{
		Protocol: "http",
		Address:  "http://localhost:9999",
	}

	_, err := Encrypt(endpoint, map[string]string{})
	// L'errore è atteso perché il server non esiste
	if err == nil {
		t.Fatal("expected error for non-existent HTTP server")
	}
}

// TestGenerateWithInvalidProtocol testa con protocollo non supportato
func TestGenerateWithInvalidProtocol(t *testing.T) {
	endpoint := securityv1alpha1.Endpoint{
		Protocol: "unknown",
		Address:  "localhost:5000",
	}

	_, err := Generate(endpoint, "password", map[string]string{})
	if err == nil {
		t.Fatal("expected error for unsupported protocol")
	}

	if err.Error() != "unsupported protocol: unknown" {
		t.Errorf("expected 'unsupported protocol: unknown', got: %s", err.Error())
	}
}

// boolPtr è una funzione helper per creare puntatori a bool
func boolPtr(b bool) *bool {
	return &b
}
