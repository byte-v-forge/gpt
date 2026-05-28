package activities

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	"github.com/google/uuid"
)

const (
	codexOAuthSentinelVersion = "20260219f9f6"
	codexOAuthSentinelReqURL  = "https://sentinel.openai.com/backend-api/sentinel/req"
	codexOAuthSentinelReferer = "https://sentinel.openai.com/backend-api/sentinel/frame.html?sv=" + codexOAuthSentinelVersion
	codexOAuthSentinelSDKURL  = "https://sentinel.openai.com/sentinel/" + codexOAuthSentinelVersion + "/sdk.js"
)

type codexOAuthSentinelGenerator struct {
	deviceID  string
	userAgent string
	sid       string
}

func codexOAuthProtocolSentinelHeader(ctx context.Context, client *GptClient, state *codexOAuthProtocolState, data map[string]any, flow string) map[string]string {
	if client == nil || state == nil {
		return nil
	}
	token, err := client.sentinelToken(ctx, flow)
	if err != nil {
		if data != nil {
			data["sentinel_error"] = codexOAuthProtocolSafeText(err.Error(), 180)
		}
		return nil
	}
	if strings.TrimSpace(token) == "" {
		return nil
	}
	if data != nil {
		data["sentinel_flow"] = flow
		data["sentinel_token_present"] = true
		var parsed map[string]any
		if json.Unmarshal([]byte(token), &parsed) == nil {
			data["sentinel_t_present"] = strings.TrimSpace(stringAny(parsed["t"])) != ""
		}
	}
	return map[string]string{"openai-sentinel-token": token}
}

func (c *GptClient) sentinelToken(ctx context.Context, flow string) (string, error) {
	if strings.TrimSpace(flow) == "" {
		flow = "authorize_continue"
	}
	deviceID := c.deviceID()
	if deviceID == "" {
		deviceID = uuid.NewString()
		if c.state != nil {
			c.state.DeviceID = deviceID
		}
	}
	if token, err := c.sentinelTokenQuickJS(ctx, deviceID, flow); err == nil && strings.TrimSpace(token) != "" {
		return token, nil
	}
	return c.sentinelTokenPure(ctx, deviceID, flow)
}

func (c *GptClient) sentinelTokenPure(ctx context.Context, deviceID, flow string) (string, error) {
	generator := newCodexOAuthSentinelGenerator(deviceID, c.userAgent())
	body, err := codexOAuthJSONNoEscape(map[string]string{
		"p":    generator.requirementsToken(),
		"id":   deviceID,
		"flow": flow,
	})
	if err != nil {
		return "", err
	}
	resp, err := c.request(ctx, fhttp.MethodPost, codexOAuthSentinelReqURL, codexOAuthSentinelReferer, false, body, map[string]string{
		"Accept":         "*/*",
		"Content-Type":   "text/plain;charset=UTF-8",
		"Sec-Fetch-Dest": "empty",
		"Sec-Fetch-Mode": "cors",
		"Sec-Fetch-Site": "same-origin",
	})
	if err != nil {
		return "", err
	}
	respBody := resp.Body
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("sentinel failed: status %d %s", resp.StatusCode, codexOAuthProtocolSafeText(string(respBody), 180))
	}
	var challenge map[string]any
	if err := json.Unmarshal(respBody, &challenge); err != nil {
		return "", fmt.Errorf("decode sentinel challenge: %w", err)
	}
	cValue := strings.TrimSpace(stringAny(challenge["token"]))
	if cValue == "" {
		return "", fmt.Errorf("sentinel token missing")
	}
	pValue := generator.requirementsToken()
	if pow, _ := challenge["proofofwork"].(map[string]any); pow != nil {
		if required, _ := boolAny(pow["required"]); required {
			seed := strings.TrimSpace(stringAny(pow["seed"]))
			if seed != "" {
				pValue = generator.proofToken(seed, strings.TrimSpace(stringAny(pow["difficulty"])))
			}
		}
	}
	payload, err := codexOAuthJSONNoEscape(struct {
		P    string `json:"p"`
		T    string `json:"t"`
		C    string `json:"c"`
		ID   string `json:"id"`
		Flow string `json:"flow"`
	}{P: pValue, T: "", C: cValue, ID: deviceID, Flow: flow})
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func newCodexOAuthSentinelGenerator(deviceID, userAgent string) codexOAuthSentinelGenerator {
	return codexOAuthSentinelGenerator{deviceID: deviceID, userAgent: userAgent, sid: uuid.NewString()}
}

func (g codexOAuthSentinelGenerator) requirementsToken() string {
	config := g.config()
	config[3] = 1
	config[9] = rand.Intn(46) + 5
	return "gAAAAAC" + codexOAuthSentinelBase64(config)
}

func (g codexOAuthSentinelGenerator) proofToken(seed, difficulty string) string {
	if difficulty == "" {
		difficulty = "0"
	}
	start := time.Now()
	config := g.config()
	for nonce := 0; nonce < 500000; nonce++ {
		config[3] = nonce
		config[9] = time.Since(start).Milliseconds()
		encoded := codexOAuthSentinelBase64(config)
		digest := codexOAuthSentinelFNV1a32(seed + encoded)
		prefix := digest
		if len(prefix) > len(difficulty) {
			prefix = prefix[:len(difficulty)]
		}
		if strings.Compare(prefix, difficulty) <= 0 {
			return "gAAAAAB" + encoded + "~S"
		}
	}
	return "gAAAAAB" + codexOAuthSentinelBase64("sentinel_pow_failed")
}

func (g codexOAuthSentinelGenerator) config() []any {
	now := time.Now().UTC().Format("Mon Jan 02 2006 15:04:05 GMT+0000 (Coordinated Universal Time)")
	perfNow := 1000 + rand.Float64()*49000
	timeOrigin := float64(time.Now().UnixMilli()) - perfNow
	navProps := []string{
		"vendorSub", "productSub", "vendor", "maxTouchPoints", "scheduling",
		"userActivation", "doNotTrack", "geolocation", "connection", "plugins",
		"mimeTypes", "pdfViewerEnabled", "webkitTemporaryStorage", "hardwareConcurrency",
		"cookieEnabled", "credentials", "mediaDevices", "permissions", "locks", "ink",
	}
	docProps := []string{"location", "implementation", "URL", "documentURI", "compatMode"}
	constructors := []string{"Object", "Function", "Array", "Number", "parseFloat", "undefined"}
	return []any{
		"1920x1080",
		now,
		4294705152,
		rand.Float64(),
		g.userAgent,
		codexOAuthSentinelSDKURL,
		nil,
		nil,
		"en-US",
		"en-US,en",
		rand.Float64(),
		navProps[rand.Intn(len(navProps))] + "−undefined",
		docProps[rand.Intn(len(docProps))],
		constructors[rand.Intn(len(constructors))],
		perfNow,
		g.sid,
		"",
		[]int{4, 8, 12, 16}[rand.Intn(4)],
		timeOrigin,
	}
}

func codexOAuthSentinelBase64(value any) string {
	body, err := codexOAuthJSONNoEscape(value)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(body)
}

func codexOAuthJSONNoEscape(value any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(buf.Bytes()), nil
}

func codexOAuthSentinelFNV1a32(text string) string {
	var h uint32 = 2166136261
	for i := 0; i < len(text); i++ {
		h ^= uint32(text[i])
		h *= 16777619
	}
	h ^= h >> 16
	h *= 2246822507
	h ^= h >> 13
	h *= 3266489909
	h ^= h >> 16
	return fmt.Sprintf("%08x", h)
}
