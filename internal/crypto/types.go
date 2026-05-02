package crypto

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
