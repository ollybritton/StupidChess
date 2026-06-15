// Package lichess connects a StupidChess engine to the Lichess Bot API: it streams incoming events,
// accepts challenges, and plays out each game by driving an engine and posting its moves.
//
// See https://lichess.org/api#tag/Bot. A bot needs an API token with the bot:play scope, created at
// https://lichess.org/account/oauth/token, on an account that has been upgraded to a BOT account
// (Client.UpgradeToBot, irreversible, only possible before the account has played any games).
package lichess

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the production Lichess host. Tests override it to point at a local server.
const DefaultBaseURL = "https://lichess.org"

// actionTimeout bounds one-shot API calls (account, move, accept, ...). Streaming calls are not
// time-limited; they live as long as their context.
const actionTimeout = 15 * time.Second

// Client is a thin wrapper over the Lichess HTTP API, authenticated with a bearer token.
type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

// NewClient builds a client for the given bot API token.
func NewClient(token string) *Client {
	return &Client{
		token:   token,
		baseURL: DefaultBaseURL,
		// No global timeout: the event and game streams are long-lived and rely on context for
		// cancellation. Per-call timeouts are applied to the one-shot actions instead.
		http: &http.Client{},
	}
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	return req, nil
}

// httpError turns a non-2xx response into an error carrying a snippet of the body.
func httpError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return fmt.Errorf("lichess %s %s: %s: %s",
		resp.Request.Method, resp.Request.URL.Path, resp.Status, strings.TrimSpace(string(body)))
}

// doAction performs a one-shot request, applies a timeout, and treats any non-2xx as an error.
func (c *Client) doAction(ctx context.Context, method, path string, form url.Values) error {
	ctx, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()

	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}

	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return httpError(resp)
	}
	return nil
}

// Account returns the authenticated account (GET /api/account).
func (c *Client) Account(ctx context.Context) (Account, error) {
	ctx, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()

	req, err := c.newRequest(ctx, http.MethodGet, "/api/account", nil)
	if err != nil {
		return Account{}, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return Account{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Account{}, httpError(resp)
	}

	var acct Account
	if err := json.NewDecoder(resp.Body).Decode(&acct); err != nil {
		return Account{}, fmt.Errorf("decoding account: %w", err)
	}
	return acct, nil
}

// stream opens a long-lived GET and returns the response body for NDJSON reading. The caller must
// close it. The request is bound to ctx, so cancelling ctx ends the stream.
func (c *Client) stream(ctx context.Context, path string) (io.ReadCloser, error) {
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, httpError(resp)
	}
	return resp.Body, nil
}

// StreamEvents opens the main event stream (GET /api/stream/event).
func (c *Client) StreamEvents(ctx context.Context) (io.ReadCloser, error) {
	return c.stream(ctx, "/api/stream/event")
}

// StreamGame opens a bot game stream (GET /api/bot/game/stream/{id}).
func (c *Client) StreamGame(ctx context.Context, gameID string) (io.ReadCloser, error) {
	return c.stream(ctx, "/api/bot/game/stream/"+gameID)
}

// MakeMove plays a UCI move in a game (POST /api/bot/game/{id}/move/{uci}).
func (c *Client) MakeMove(ctx context.Context, gameID, uci string) error {
	return c.doAction(ctx, http.MethodPost, "/api/bot/game/"+gameID+"/move/"+uci, nil)
}

// AcceptChallenge accepts an incoming challenge (POST /api/challenge/{id}/accept).
func (c *Client) AcceptChallenge(ctx context.Context, challengeID string) error {
	return c.doAction(ctx, http.MethodPost, "/api/challenge/"+challengeID+"/accept", nil)
}

// DeclineChallenge declines an incoming challenge with a reason (POST /api/challenge/{id}/decline).
func (c *Client) DeclineChallenge(ctx context.Context, challengeID, reason string) error {
	form := url.Values{}
	if reason != "" {
		form.Set("reason", reason)
	}
	return c.doAction(ctx, http.MethodPost, "/api/challenge/"+challengeID+"/decline", form)
}

// Chat posts a chat message in a game (POST /api/bot/game/{id}/chat). room is "player" or "spectator".
func (c *Client) Chat(ctx context.Context, gameID, room, text string) error {
	form := url.Values{}
	form.Set("room", room)
	form.Set("text", text)
	return c.doAction(ctx, http.MethodPost, "/api/bot/game/"+gameID+"/chat", form)
}

// UpgradeToBot irreversibly turns the token's account into a BOT account
// (POST /api/bot/account/upgrade). It only works on an account that has never played a game.
func (c *Client) UpgradeToBot(ctx context.Context) error {
	return c.doAction(ctx, http.MethodPost, "/api/bot/account/upgrade", nil)
}

// OnlineBots returns up to nb currently-online bots (GET /api/bot/online), as usernames. These are the
// candidates to challenge so the bot is always playing.
func (c *Client) OnlineBots(ctx context.Context, nb int) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()

	body, err := c.stream(ctx, fmt.Sprintf("/api/bot/online?nb=%d", nb))
	if err != nil {
		return nil, err
	}
	defer body.Close()

	var bots []string
	err = streamNDJSON(body, func(line []byte) error {
		var u struct {
			Username string `json:"username"`
		}
		if json.Unmarshal(line, &u) == nil && u.Username != "" {
			bots = append(bots, u.Username)
		}
		return nil
	})
	return bots, err
}

// ChallengeParams describes a challenge to send to another player.
type ChallengeParams struct {
	Rated          bool
	ClockLimit     time.Duration // initial time
	ClockIncrement time.Duration // per-move increment
	Color          string        // "random", "white" or "black" (default "random")
	Variant        string        // e.g. "standard" (default standard)
}

// Challenge challenges a user to a game (POST /api/challenge/{username}). It returns once the challenge
// is created; acceptance arrives later as a gameStart event.
func (c *Client) Challenge(ctx context.Context, username string, p ChallengeParams) error {
	form := url.Values{}
	form.Set("rated", strconv.FormatBool(p.Rated))
	form.Set("clock.limit", strconv.Itoa(int(p.ClockLimit.Seconds())))
	form.Set("clock.increment", strconv.Itoa(int(p.ClockIncrement.Seconds())))
	if p.Color != "" {
		form.Set("color", p.Color)
	}
	if p.Variant != "" {
		form.Set("variant", p.Variant)
	}
	return c.doAction(ctx, http.MethodPost, "/api/challenge/"+username, form)
}

// streamNDJSON reads a Lichess NDJSON stream line by line, skipping the blank keep-alive lines, and
// passes each non-empty line to handle. It stops (returning the handler's error) the first time handle
// returns non-nil, or when the stream ends.
func streamNDJSON(r io.Reader, handle func(line []byte) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue // keep-alive ping
		}
		if err := handle(line); err != nil {
			return err
		}
	}
	return scanner.Err()
}
