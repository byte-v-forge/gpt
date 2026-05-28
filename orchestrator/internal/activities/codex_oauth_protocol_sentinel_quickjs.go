package activities

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
)

//go:embed openai_sentinel_quickjs.js
var codexOAuthSentinelQuickJSScript string

const codexOAuthSentinelNodeWrapper = `
const fs = require('fs');
const timeoutMs = Number(process.env.OPENAI_SENTINEL_VM_TIMEOUT_MS || '30000');
const sdkFile = process.env.OPENAI_SENTINEL_SDK_FILE;
const scriptFile = process.env.OPENAI_SENTINEL_QUICKJS_SCRIPT;
let input = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', (chunk) => { input += chunk; });
process.stdin.on('end', async () => {
  try {
    const payload = JSON.parse(input || '{}');
    globalThis.__payload_json = JSON.stringify(payload);
    globalThis.__sdk_source = fs.readFileSync(sdkFile, 'utf8');
    globalThis.__vm_done = false;
    globalThis.__vm_output_json = '';
    globalThis.__vm_error = '';
    eval(fs.readFileSync(scriptFile, 'utf8'));
    const started = Date.now();
    while (!globalThis.__vm_done) {
      if ((Date.now() - started) > timeoutMs) throw new Error('QuickJS script timeout');
      await new Promise((resolve) => setTimeout(resolve, 1));
    }
    if (String(globalThis.__vm_error || '').trim()) throw new Error(String(globalThis.__vm_error));
    process.stdout.write(String(globalThis.__vm_output_json || ''));
  } catch (err) {
    process.stderr.write(err && err.stack ? String(err.stack) : String(err));
    process.exit(1);
  }
});
`

func (c *GptClient) sentinelTokenQuickJS(ctx context.Context, deviceID, flow string) (string, error) {
	sdkFile, scriptFile, err := c.prepareSentinelQuickJSFiles(ctx)
	if err != nil {
		return "", err
	}
	requirements, err := runCodexOAuthSentinelQuickJSAction(ctx, sdkFile, scriptFile, map[string]any{
		"action":    "requirements",
		"device_id": deviceID,
	})
	if err != nil {
		return "", err
	}
	requestP := strings.TrimSpace(stringAny(requirements["request_p"]))
	if requestP == "" {
		return "", fmt.Errorf("sentinel quickjs requirements missing request_p")
	}
	challenge, err := c.fetchSentinelChallenge(ctx, deviceID, flow, requestP)
	if err != nil {
		return "", err
	}
	cValue := strings.TrimSpace(stringAny(challenge["token"]))
	if cValue == "" {
		return "", fmt.Errorf("sentinel quickjs challenge token missing")
	}
	solved, err := runCodexOAuthSentinelQuickJSAction(ctx, sdkFile, scriptFile, map[string]any{
		"action":    "solve",
		"device_id": deviceID,
		"request_p": requestP,
		"challenge": challenge,
	})
	if err != nil {
		return "", err
	}
	finalP := strings.TrimSpace(stringAny(solved["final_p"]))
	if finalP == "" {
		finalP = strings.TrimSpace(stringAny(solved["p"]))
	}
	tValue := strings.TrimSpace(stringAny(solved["t"]))
	if finalP == "" || tValue == "" {
		return "", fmt.Errorf("sentinel quickjs solve missing final token")
	}
	payload, err := codexOAuthJSONNoEscape(struct {
		P    string `json:"p"`
		T    string `json:"t"`
		C    string `json:"c"`
		ID   string `json:"id"`
		Flow string `json:"flow"`
	}{P: finalP, T: tValue, C: cValue, ID: deviceID, Flow: flow})
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func (c *GptClient) prepareSentinelQuickJSFiles(ctx context.Context) (string, string, error) {
	dir := filepath.Join(os.TempDir(), "byte-v-forge-openai-sentinel", codexOAuthSentinelVersion)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	sdkFile := filepath.Join(dir, "sdk.js")
	if info, err := os.Stat(sdkFile); err != nil || info.Size() == 0 {
		resp, err := c.fetchSentinelSDK(ctx)
		if err != nil {
			return "", "", err
		}
		if err := os.WriteFile(sdkFile, resp, 0o600); err != nil {
			return "", "", err
		}
	}
	scriptFile := filepath.Join(dir, "openai_sentinel_quickjs.js")
	if err := os.WriteFile(scriptFile, []byte(codexOAuthSentinelQuickJSScript), 0o600); err != nil {
		return "", "", err
	}
	return sdkFile, scriptFile, nil
}

func (c *GptClient) fetchSentinelSDK(ctx context.Context) ([]byte, error) {
	resp, err := c.request(ctx, fhttp.MethodGet, codexOAuthSentinelSDKURL, "https://auth.openai.com/", false, nil, map[string]string{
		"Accept":         "*/*",
		"Sec-Fetch-Dest": "script",
		"Sec-Fetch-Mode": "no-cors",
		"Sec-Fetch-Site": "same-site",
	})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 || len(resp.Body) == 0 {
		return nil, fmt.Errorf("download sentinel sdk failed: status %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func (c *GptClient) fetchSentinelChallenge(ctx context.Context, deviceID, flow, requestP string) (map[string]any, error) {
	body, err := codexOAuthJSONNoEscape(map[string]string{"p": requestP, "id": deviceID, "flow": flow})
	if err != nil {
		return nil, err
	}
	resp, err := c.request(ctx, fhttp.MethodPost, codexOAuthSentinelReqURL, codexOAuthSentinelReferer, false, body, map[string]string{
		"Accept":         "*/*",
		"Content-Type":   "text/plain;charset=UTF-8",
		"Sec-Fetch-Dest": "empty",
		"Sec-Fetch-Mode": "cors",
		"Sec-Fetch-Site": "same-origin",
	})
	if err != nil {
		return nil, err
	}
	respBody := resp.Body
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("sentinel quickjs challenge failed: status %d %s", resp.StatusCode, codexOAuthProtocolSafeText(string(respBody), 180))
	}
	var challenge map[string]any
	if err := json.Unmarshal(respBody, &challenge); err != nil {
		return nil, fmt.Errorf("decode sentinel quickjs challenge: %w", err)
	}
	return challenge, nil
}

func runCodexOAuthSentinelQuickJSAction(ctx context.Context, sdkFile, scriptFile string, payload map[string]any) (map[string]any, error) {
	body, err := codexOAuthJSONNoEscape(payload)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithTimeout(ctx, 55*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "node", "-e", codexOAuthSentinelNodeWrapper)
	cmd.Env = append(os.Environ(),
		"OPENAI_SENTINEL_SDK_FILE="+sdkFile,
		"OPENAI_SENTINEL_QUICKJS_SCRIPT="+scriptFile,
		"OPENAI_SENTINEL_VM_TIMEOUT_MS=30000",
	)
	cmd.Stdin = bytes.NewReader(body)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("sentinel quickjs node failed: %w: %s", err, codexOAuthProtocolSafeText(stderr.String(), 220))
	}
	var out map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &out); err != nil {
		return nil, fmt.Errorf("decode sentinel quickjs output: %w", err)
	}
	return out, nil
}
