package crypto

import (
	"bytes"
	"encoding/json"
	"net/http"
)

type EncryptRequest struct {
	Data map[string]string `json:"data"`
}

type EncryptResponse struct {
	Ciphertext map[string]string `json:"ciphertext"`
}

func Encrypt(endpoint string, data map[string]string) (map[string]string, error) {

	reqBody := EncryptRequest{
		Data: data,
	}

	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	var encResp EncryptResponse

	err = json.NewDecoder(resp.Body).Decode(&encResp)
	if err != nil {
		return nil, err
	}

	return encResp.Ciphertext, nil
}
