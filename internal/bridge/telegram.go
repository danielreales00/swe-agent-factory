package bridge

import (
	"context"
	"fmt"
	"log"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Message is one inbound Telegram message after allowlist filtering. The
// bridge's session router consumes these.
type Message struct {
	UserID   int64  // Telegram numeric user id
	Username string // optional, may be empty
	ChatID   int64  // for DMs == UserID; for groups, the group id
	Text     string // raw message body
	UpdateID int    // Telegram update id (for diagnostics)
}

// Handler receives one inbound message. Called from the long-poll goroutine,
// so handlers should either return quickly or hand work off asynchronously.
type Handler func(ctx context.Context, msg Message)

// Client is a thin Telegram bot wrapper: connects, long-polls inbound
// updates, filters by allowlist, dispatches to a Handler, and provides
// outbound message sending.
type Client struct {
	api       *tg.BotAPI
	allowlist *Allowlist
	logger    *log.Logger
}

// NewClient connects to Telegram with the given bot token. The token is
// validated by an immediate getMe call; an invalid token surfaces here.
func NewClient(token string, allowlist *Allowlist, logger *log.Logger) (*Client, error) {
	if logger == nil {
		logger = log.Default()
	}
	api, err := tg.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("connect telegram: %w", err)
	}
	return &Client{api: api, allowlist: allowlist, logger: logger}, nil
}

// Self returns the bot's @username and numeric id. Useful for startup logs.
func (c *Client) Self() (string, int64) {
	return c.api.Self.UserName, c.api.Self.ID
}

// Run long-polls Telegram, filters by allowlist, and dispatches matching
// messages to the handler. Returns when ctx is cancelled or the updates
// channel closes.
//
// Non-allowlisted messages are logged at debug level and dropped — no
// reply, no error surface, to avoid leaking the bot's existence to
// uninvited users.
func (c *Client) Run(ctx context.Context, handler Handler) error {
	upd := tg.NewUpdate(0)
	upd.Timeout = 30 // long-poll seconds
	updates := c.api.GetUpdatesChan(upd)
	defer c.api.StopReceivingUpdates()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case u, ok := <-updates:
			if !ok {
				return nil
			}
			if u.Message == nil || u.Message.From == nil {
				continue
			}
			from := u.Message.From
			if !c.allowlist.Has(from.ID) {
				c.logger.Printf("ignoring message from non-allowlisted user_id=%d username=%q",
					from.ID, from.UserName)
				continue
			}
			handler(ctx, Message{
				UserID:   from.ID,
				Username: from.UserName,
				ChatID:   u.Message.Chat.ID,
				Text:     u.Message.Text,
				UpdateID: u.UpdateID,
			})
		}
	}
}

// Send posts a plain-text message to the given chat. Returns the new
// message's Telegram id on success.
func (c *Client) Send(chatID int64, text string) (int, error) {
	msg := tg.NewMessage(chatID, text)
	sent, err := c.api.Send(msg)
	if err != nil {
		return 0, err
	}
	return sent.MessageID, nil
}
