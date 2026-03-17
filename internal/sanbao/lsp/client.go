// ABOUTME: LSP JSON-RPC client that spawns language servers and queries diagnostics.
// ABOUTME: Supports Content-Length framed JSON-RPC 2.0 over stdio — the standard LSP transport.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Client manages a single language server process and communicates with it
// over JSON-RPC 2.0 using the LSP Content-Length framing protocol.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	writeMu          sync.Mutex // serializes writes to stdin
	stateMu          sync.Mutex // guards nextID, pending, and publishListeners
	nextID           int64
	pending          map[int64]chan json.RawMessage    // request ID → response channel
	publishListeners map[string][]chan []lspDiagnostic // file URI → listener channels

	done chan struct{} // closed when reader loop exits
}

// Diagnostic represents a single diagnostic entry returned by the language
// server for a specific file location.
type Diagnostic struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"` // "error", "warning", "info", "hint"
	Message  string `json:"message"`
}

// jsonRPCRequest is an outbound JSON-RPC 2.0 request or notification.
// Requests include a non-zero ID; notifications omit it (encoded as
// omitempty, which drops zero values — we use 0 for notifications).
type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

// jsonRPCResponse is an inbound JSON-RPC 2.0 response or server notification.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonRPCError is the error object in a JSON-RPC error response.
type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// lspPosition is the LSP Position type (zero-based line and character).
type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// lspRange is the LSP Range type.
type lspRange struct {
	Start lspPosition `json:"start"`
	End   lspPosition `json:"end"`
}

// lspDiagnostic is the LSP Diagnostic object received from the server.
type lspDiagnostic struct {
	Range    lspRange `json:"range"`
	Severity *int     `json:"severity,omitempty"` // 1=Error 2=Warning 3=Info 4=Hint
	Message  string   `json:"message"`
}

// lspDocumentDiagnosticReport is the response body for textDocument/diagnostic.
type lspDocumentDiagnosticReport struct {
	Kind  string          `json:"kind"` // "full" or "unchanged"
	Items []lspDiagnostic `json:"items"`
}

// lspPublishDiagnosticsParams is the params block for textDocument/publishDiagnostics.
type lspPublishDiagnosticsParams struct {
	URI         string          `json:"uri"`
	Diagnostics []lspDiagnostic `json:"diagnostics"`
}

// NewClient spawns the language server process identified by serverCmd, sends
// the LSP initialize request with repoRoot as the workspace root, waits for
// the handshake to complete, and returns a ready-to-use Client.
//
// repoRoot must be an absolute path. Returns an error if the binary is not
// found, the process fails to start, or initialization times out.
func NewClient(serverCmd string, repoRoot string) (*Client, error) {
	binPath, err := exec.LookPath(serverCmd)
	if err != nil {
		return nil, fmt.Errorf("lsp: server binary %q not found: %w", serverCmd, err)
	}

	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("lsp: abs path for repoRoot: %w", err)
	}

	cmd := exec.Command(binPath)
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp: stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp: stdout pipe: %w", err)
	}
	// Discard stderr — gopls writes progress and log messages there.
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lsp: start %q: %w", serverCmd, err)
	}

	c := &Client{
		cmd:              cmd,
		stdin:            stdinPipe,
		stdout:           bufio.NewReaderSize(stdoutPipe, 1<<20),
		pending:          make(map[int64]chan json.RawMessage),
		publishListeners: make(map[string][]chan []lspDiagnostic),
		done:             make(chan struct{}),
	}

	// Start the reader goroutine that dispatches responses and notifications.
	go c.readLoop()

	// Send LSP initialize request.
	rootURI := pathToURI(absRoot)
	initParams := map[string]interface{}{
		"processId":        nil,
		"rootUri":          rootURI,
		"workspaceFolders": []map[string]string{{"uri": rootURI, "name": "root"}},
		"capabilities": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"diagnostic": map[string]interface{}{
					"dynamicRegistration": false,
				},
				"synchronization": map[string]interface{}{
					"dynamicRegistration": false,
				},
			},
			"workspace": map[string]interface{}{
				"workspaceFolders": true,
			},
		},
	}

	if _, err := c.callWithTimeout("initialize", initParams, 30*time.Second); err != nil {
		_ = c.kill()
		return nil, fmt.Errorf("lsp: initialize: %w", err)
	}

	// Send initialized notification — no response expected.
	if err := c.notify("initialized", map[string]interface{}{}); err != nil {
		_ = c.kill()
		return nil, fmt.Errorf("lsp: initialized notification: %w", err)
	}

	return c, nil
}

// Diagnostics opens the file at filePath in the language server, requests
// diagnostics, and returns the result. filePath must be an absolute path.
//
// It first subscribes to textDocument/publishDiagnostics notifications, then
// sends textDocument/didOpen, then tries the pull-model textDocument/diagnostic
// request (LSP 3.17). If the pull model fails or returns no useful response,
// it waits for a push notification from the server.
func (c *Client) Diagnostics(filePath string) ([]Diagnostic, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("lsp: abs path: %w", err)
	}
	fileURI := pathToURI(absPath)

	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("lsp: read file %s: %w", absPath, err)
	}

	// Subscribe to publishDiagnostics BEFORE sending didOpen so we don't miss
	// a notification that arrives immediately after the request.
	pubCh := make(chan []lspDiagnostic, 4)
	c.stateMu.Lock()
	c.publishListeners[fileURI] = append(c.publishListeners[fileURI], pubCh)
	c.stateMu.Unlock()

	defer func() {
		c.stateMu.Lock()
		listeners := c.publishListeners[fileURI]
		for i, ch := range listeners {
			if ch == pubCh {
				c.publishListeners[fileURI] = append(listeners[:i], listeners[i+1:]...)
				break
			}
		}
		c.stateMu.Unlock()
	}()

	// Send textDocument/didOpen notification.
	if err := c.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        fileURI,
			"languageId": "go",
			"version":    1,
			"text":       string(content),
		},
	}); err != nil {
		return nil, fmt.Errorf("lsp: didOpen: %w", err)
	}

	// Attempt pull diagnostics (LSP 3.17 / gopls >= 0.14).
	pullResult, pullErr := c.callWithTimeout("textDocument/diagnostic", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": fileURI},
	}, 15*time.Second)

	if pullErr == nil {
		var report lspDocumentDiagnosticReport
		if err := json.Unmarshal(pullResult, &report); err == nil && report.Kind == "full" {
			return convertDiagnostics(absPath, report.Items), nil
		}
	}

	// Fall back to push diagnostics: wait for textDocument/publishDiagnostics.
	select {
	case items := <-pubCh:
		return convertDiagnostics(absPath, items), nil
	case <-time.After(15 * time.Second):
		// Return empty rather than error: the server may simply have no
		// diagnostics to report and sends nothing.
		return []Diagnostic{}, nil
	}
}

// Shutdown sends the LSP shutdown request followed by the exit notification,
// then waits for the server process to terminate. If the server does not exit
// within a timeout, it is killed forcibly.
func (c *Client) Shutdown() error {
	_, err := c.callWithTimeout("shutdown", map[string]interface{}{}, 10*time.Second)
	if err != nil {
		_ = c.notify("exit", nil)
		_ = c.kill()
		return fmt.Errorf("lsp: shutdown request: %w", err)
	}

	if err := c.notify("exit", nil); err != nil {
		_ = c.kill()
		return fmt.Errorf("lsp: exit notification: %w", err)
	}

	// Close stdin so the server sees EOF.
	_ = c.stdin.Close()

	// Wait for the process to exit, with a graceful timeout.
	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()

	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		_ = c.kill()
		return nil // killed cleanly
	}
}

// readLoop runs in a background goroutine and dispatches all messages arriving
// on stdout to their waiting callers or notification handlers.
func (c *Client) readLoop() {
	defer close(c.done)
	for {
		msg, err := readMessage(c.stdout)
		if err != nil {
			break
		}

		var resp jsonRPCResponse
		if err := json.Unmarshal(msg, &resp); err != nil {
			continue
		}

		if resp.ID != nil {
			// Response to one of our requests.
			c.stateMu.Lock()
			ch, ok := c.pending[*resp.ID]
			if ok {
				delete(c.pending, *resp.ID)
			}
			c.stateMu.Unlock()
			if ok {
				if resp.Error != nil {
					errPayload, _ := json.Marshal(map[string]interface{}{
						"__lspError": resp.Error.Message,
					})
					ch <- errPayload
				} else {
					ch <- resp.Result
				}
			}
		} else if resp.Method != "" {
			// Server-initiated notification.
			c.handleNotification(resp.Method, resp.Params)
		}
	}

	// Drain all pending waiters so they are not left blocked.
	c.stateMu.Lock()
	for id, ch := range c.pending {
		delete(c.pending, id)
		close(ch)
	}
	c.stateMu.Unlock()
}

// handleNotification processes server-sent notifications, specifically
// textDocument/publishDiagnostics.
func (c *Client) handleNotification(method string, params json.RawMessage) {
	if method != "textDocument/publishDiagnostics" {
		return
	}
	var p lspPublishDiagnosticsParams
	if err := json.Unmarshal(params, &p); err != nil {
		return
	}
	c.stateMu.Lock()
	listeners := make([]chan []lspDiagnostic, len(c.publishListeners[p.URI]))
	copy(listeners, c.publishListeners[p.URI])
	c.stateMu.Unlock()
	for _, ch := range listeners {
		select {
		case ch <- p.Diagnostics:
		default:
		}
	}
}

// callWithTimeout sends a JSON-RPC request and waits up to timeout for the
// response. Returns the raw JSON result or an error.
func (c *Client) callWithTimeout(method string, params interface{}, timeout time.Duration) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.nextID, 1)
	ch := make(chan json.RawMessage, 1)

	c.stateMu.Lock()
	c.pending[id] = ch
	c.stateMu.Unlock()

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	if err := c.writeMessage(req); err != nil {
		c.stateMu.Lock()
		delete(c.pending, id)
		c.stateMu.Unlock()
		return nil, fmt.Errorf("write %s: %w", method, err)
	}

	select {
	case raw, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("server closed while waiting for %s response", method)
		}
		// Check for our encoded error sentinel.
		var errCheck map[string]json.RawMessage
		if json.Unmarshal(raw, &errCheck) == nil {
			if errMsg, ok := errCheck["__lspError"]; ok {
				var msg string
				_ = json.Unmarshal(errMsg, &msg)
				return nil, fmt.Errorf("server error for %s: %s", method, msg)
			}
		}
		return raw, nil
	case <-time.After(timeout):
		c.stateMu.Lock()
		delete(c.pending, id)
		c.stateMu.Unlock()
		return nil, fmt.Errorf("timeout waiting for %s response after %s", method, timeout)
	}
}

// notify sends a JSON-RPC notification (no ID field; no response expected).
func (c *Client) notify(method string, params interface{}) error {
	req := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return c.writeMessage(req)
}

// writeMessage serializes req as JSON and writes it with Content-Length framing.
// It holds writeMu to prevent interleaved messages on stdin.
func (c *Client) writeMessage(req jsonRPCRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if _, err := io.WriteString(c.stdin, header); err != nil {
		return err
	}
	_, err = c.stdin.Write(body)
	return err
}

// readMessage reads one Content-Length framed message from r.
func readMessage(r *bufio.Reader) ([]byte, error) {
	contentLength := -1

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length: ") {
			val := strings.TrimPrefix(line, "Content-Length: ")
			n, err := strconv.Atoi(strings.TrimSpace(val))
			if err != nil {
				return nil, fmt.Errorf("bad Content-Length value %q: %w", val, err)
			}
			contentLength = n
		}
	}

	if contentLength < 0 {
		return nil, fmt.Errorf("no Content-Length header in LSP message")
	}

	buf := make([]byte, contentLength)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("read LSP body: %w", err)
	}
	return buf, nil
}

// pathToURI converts an absolute filesystem path to a file:// URI.
// Uses url.URL so that path components are correctly percent-encoded and
// slashes are preserved (giving file:///path/to/file, not file://%2Fpath...).
func pathToURI(absPath string) string {
	u := &url.URL{
		Scheme: "file",
		Path:   absPath,
	}
	return u.String()
}

// kill forcibly terminates the server process.
func (c *Client) kill() error {
	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}
	return nil
}

// convertDiagnostics converts LSP diagnostic objects to the package Diagnostic
// type. LSP line numbers are zero-based; Diagnostic.Line is one-based.
func convertDiagnostics(filePath string, items []lspDiagnostic) []Diagnostic {
	result := make([]Diagnostic, 0, len(items))
	for _, item := range items {
		result = append(result, Diagnostic{
			File:     filePath,
			Line:     item.Range.Start.Line + 1,
			Severity: severityString(item.Severity),
			Message:  item.Message,
		})
	}
	return result
}

// severityString maps the LSP integer severity to the string used by
// Diagnostic.Severity. nil severity defaults to "warning".
func severityString(severity *int) string {
	if severity == nil {
		return "warning"
	}
	switch *severity {
	case 1:
		return "error"
	case 2:
		return "warning"
	case 3:
		return "info"
	case 4:
		return "hint"
	default:
		return "warning"
	}
}
