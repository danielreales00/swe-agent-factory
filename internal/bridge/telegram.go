package bridge

import (
	"context"
	"fmt"
	"log"

	tg "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Message is one inbound Telegram message after allowlist filtering.
type Message struct {
	UserID   int64  // Telegram numeric user id
	Username string // optional, may be empty
	ChatID   int64  // for DMs == UserID; for groups, the group id
	Text     string // raw message body
	UpdateID int    // Telegram update id (for diagnostics)
}

// Callback is one inbound Telegram callback query — emitted when an
// allowlisted user taps an inline-keyboard button.
type Callback struct {
	UserID   int64
	Username string
	ChatID   int64  // chat that owns the message with the button
	MsgID    int    // the message that hosts the inline keyboard
	Data     string // the button's callback_data payload
	QueryID  string // pass to AnswerCallback to dismiss the spinner
	UpdateID int
}

// Button is one inline-keyboard button on a SendWithButtons message.
type Button struct {
	Label string // user-visible
	Data  string // returned in Callback.Data on tap; <= 64 bytes
}

// MessageHandler receives one inbound Telegram message. Called from the
// long-poll goroutine, so handlers should either return quickly or hand
// work off asynchronously.
type MessageHandler func(ctx context.Context, msg Message)

// CallbackHandler receives one inbound Telegram callback query (button tap).
type CallbackHandler func(ctx context.Context, cb Callback)

// Handlers bundles the dispatch callbacks for Client.Run. Either field may
// be nil to ignore that event class.
type Handlers struct {
	OnMessage  MessageHandler
	OnCallback CallbackHandler
}

// Client is a thin Telegram bot wrapper.
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

// Self returns the bot's @username and numeric id.
func (c *Client) Self() (string, int64) {
	return c.api.Self.UserName, c.api.Self.ID
}

// Run long-polls Telegram and dispatches allowlisted messages and callback
// queries to the handlers. Returns when ctx is cancelled or the updates
// channel closes.
//
// Non-allowlisted users are silently ignored for both messages and callbacks
// (no reply, no AnswerCallback) so the bot's existence isn't leaked to
// uninvited users.
func (c *Client) Run(ctx context.Context, h Handlers) error {
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
			switch {
			case u.Message != nil && u.Message.From != nil:
				c.dispatchMessage(ctx, h.OnMessage, u)
			case u.CallbackQuery != nil && u.CallbackQuery.From != nil:
				c.dispatchCallback(ctx, h.OnCallback, u)
			}
		}
	}
}

func (c *Client) dispatchMessage(ctx context.Context, h MessageHandler, u tg.Update) {
	from := u.Message.From
	if !c.allowlist.Has(from.ID) {
		c.logger.Printf("ignoring message from non-allowlisted user_id=%d username=%q",
			from.ID, from.UserName)
		return
	}
	if h == nil {
		return
	}
	h(ctx, Message{
		UserID:   from.ID,
		Username: from.UserName,
		ChatID:   u.Message.Chat.ID,
		Text:     u.Message.Text,
		UpdateID: u.UpdateID,
	})
}

func (c *Client) dispatchCallback(ctx context.Context, h CallbackHandler, u tg.Update) {
	q := u.CallbackQuery
	if !c.allowlist.Has(q.From.ID) {
		c.logger.Printf("ignoring callback from non-allowlisted user_id=%d", q.From.ID)
		return
	}
	if h == nil {
		return
	}
	var chatID int64
	var msgID int
	if q.Message != nil {
		chatID = q.Message.Chat.ID
		msgID = q.Message.MessageID
	}
	h(ctx, Callback{
		UserID:   q.From.ID,
		Username: q.From.UserName,
		ChatID:   chatID,
		MsgID:    msgID,
		Data:     q.Data,
		QueryID:  q.ID,
		UpdateID: u.UpdateID,
	})
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

// SendWithButtons sends a message with a single row of inline-keyboard
// buttons. Returns the new message's Telegram id; callers persist it so
// they can edit the message later (e.g. to strip the buttons after a tap).
func (c *Client) SendWithButtons(chatID int64, text string, buttons []Button) (int, error) {
	msg := tg.NewMessage(chatID, text)
	row := make([]tg.InlineKeyboardButton, 0, len(buttons))
	for _, b := range buttons {
		row = append(row, tg.NewInlineKeyboardButtonData(b.Label, b.Data))
	}
	msg.ReplyMarkup = tg.NewInlineKeyboardMarkup(tg.NewInlineKeyboardRow(row...))
	sent, err := c.api.Send(msg)
	if err != nil {
		return 0, err
	}
	return sent.MessageID, nil
}

// EditMessage rewrites an existing bot message's text and removes any
// inline keyboard it had.
func (c *Client) EditMessage(chatID int64, msgID int, text string) error {
	edit := tg.NewEditMessageText(chatID, msgID, text)
	_, err := c.api.Send(edit)
	return err
}

// AnswerCallback dismisses the spinner on a button tap. Optional `text`
// is shown as a brief toast.
func (c *Client) AnswerCallback(queryID, text string) error {
	cb := tg.NewCallback(queryID, text)
	_, err := c.api.Request(cb)
	return err
}
