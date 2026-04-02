package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const apiBase = "https://api.telegram.org/bot"

// Client wraps the Telegram Bot API with the methods this service needs.
type Client struct {
	token              string
	chatID             int64
	botUserID          int64
	reactionsSupported bool
	http               *http.Client
}

func NewClient(token string, chatID int64) *Client {
	return &Client{
		token:  token,
		chatID: chatID,
		http: &http.Client{
			Timeout: 70 * time.Second, // Must be > long-poll timeout (60s)
		},
	}
}

// --- API response types ---

type APIResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description,omitempty"`
}

type BotInfo struct {
	ID                      int64  `json:"id"`
	IsBot                   bool   `json:"is_bot"`
	FirstName               string `json:"first_name"`
	Username                string `json:"username"`
	CanReadAllGroupMessages bool   `json:"can_read_all_group_messages"`
}

type WebhookInfo struct {
	URL string `json:"url"`
}

type ChatInfo struct {
	ID               int64  `json:"id"`
	Type             string `json:"type"`
	MaxReactionCount int    `json:"max_reaction_count"`
}

type ChatMember struct {
	User   BotInfo `json:"user"`
	Status string  `json:"status"`
}

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

type Message struct {
	MessageID int       `json:"message_id"`
	From      *UserInfo `json:"from,omitempty"`
	Chat      *Chat     `json:"chat"`
	Date      int64     `json:"date"`
	Text      string    `json:"text"`
	Entities  []Entity  `json:"entities,omitempty"`
	ReplyTo   *Message  `json:"reply_to_message,omitempty"`
}

type UserInfo struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

type Chat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
}

type Entity struct {
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	Type   string `json:"type"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    *UserInfo `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data"`
}

type InlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

// --- API methods ---

func (c *Client) GetMe() (*BotInfo, error) {
	resp, err := c.get("getMe")
	if err != nil {
		return nil, err
	}
	var info BotInfo
	if err := json.Unmarshal(resp.Result, &info); err != nil {
		return nil, fmt.Errorf("parse getMe: %w", err)
	}
	c.botUserID = info.ID
	return &info, nil
}

func (c *Client) GetWebhookInfo() (*WebhookInfo, error) {
	resp, err := c.get("getWebhookInfo")
	if err != nil {
		return nil, err
	}
	var info WebhookInfo
	if err := json.Unmarshal(resp.Result, &info); err != nil {
		return nil, fmt.Errorf("parse getWebhookInfo: %w", err)
	}
	return &info, nil
}

func (c *Client) GetChat() (*ChatInfo, error) {
	resp, err := c.get(fmt.Sprintf("getChat?chat_id=%d", c.chatID))
	if err != nil {
		return nil, err
	}
	var info ChatInfo
	if err := json.Unmarshal(resp.Result, &info); err != nil {
		return nil, fmt.Errorf("parse getChat: %w", err)
	}
	return &info, nil
}

func (c *Client) GetChatMember(userID int64) (*ChatMember, error) {
	resp, err := c.get(fmt.Sprintf("getChatMember?chat_id=%d&user_id=%d", c.chatID, userID))
	if err != nil {
		return nil, err
	}
	var member ChatMember
	if err := json.Unmarshal(resp.Result, &member); err != nil {
		return nil, fmt.Errorf("parse getChatMember: %w", err)
	}
	return &member, nil
}

// GetUpdates long-polls for new messages. Blocks up to timeoutSec.
func (c *Client) GetUpdates(offset int64, timeoutSec int) ([]Update, error) {
	url := fmt.Sprintf("getUpdates?offset=%d&timeout=%d&allowed_updates=[\"message\",\"callback_query\"]", offset, timeoutSec)
	resp, err := c.get(url)
	if err != nil {
		return nil, err
	}
	var updates []Update
	if err := json.Unmarshal(resp.Result, &updates); err != nil {
		return nil, fmt.Errorf("parse getUpdates: %w", err)
	}
	return updates, nil
}

// SetReaction adds an emoji reaction. Non-fatal on failure.
func (c *Client) SetReaction(messageID int, emoji string) bool {
	if !c.reactionsSupported {
		return false
	}
	body := map[string]interface{}{
		"chat_id":    c.chatID,
		"message_id": messageID,
		"reaction":   []map[string]string{{"type": "emoji", "emoji": emoji}},
	}
	_, err := c.post("setMessageReaction", body)
	if err != nil {
		fmt.Printf("[telegram] setReaction(%s) on message %d FAILED: %v\n", emoji, messageID, err)
	}
	return err == nil
}

func (c *Client) SendReply(replyToMsgID int, html string) (int, error) {
	if len(html) > 4090 {
		html = html[:4087] + "..."
	}
	body := map[string]interface{}{
		"chat_id":             c.chatID,
		"reply_to_message_id": replyToMsgID,
		"text":                html,
		"parse_mode":          "HTML",
	}
	resp, err := c.post("sendMessage", body)
	if err != nil {
		return 0, err
	}
	var msg Message
	if err := json.Unmarshal(resp.Result, &msg); err != nil {
		return 0, fmt.Errorf("parse sendMessage: %w", err)
	}
	return msg.MessageID, nil
}

func (c *Client) SendReplyWithButtons(replyToMsgID int, html string, buttons []InlineButton) (int, error) {
	if len(html) > 4090 {
		html = html[:4087] + "..."
	}
	var keyboardRow []map[string]string
	for _, btn := range buttons {
		keyboardRow = append(keyboardRow, map[string]string{
			"text":          btn.Text,
			"callback_data": btn.CallbackData,
		})
	}
	body := map[string]interface{}{
		"chat_id":    c.chatID,
		"text":       html,
		"parse_mode": "HTML",
		"reply_markup": map[string]interface{}{
			"inline_keyboard": []interface{}{keyboardRow},
		},
	}
	if replyToMsgID > 0 {
		body["reply_to_message_id"] = replyToMsgID
	}
	resp, err := c.post("sendMessage", body)
	if err != nil {
		return 0, err
	}
	var msg Message
	if err := json.Unmarshal(resp.Result, &msg); err != nil {
		return 0, fmt.Errorf("parse sendMessage: %w", err)
	}
	return msg.MessageID, nil
}

func (c *Client) SendMessage(html string) (int, error) {
	if len(html) > 4090 {
		html = html[:4087] + "..."
	}
	body := map[string]interface{}{
		"chat_id":    c.chatID,
		"text":       html,
		"parse_mode": "HTML",
	}
	resp, err := c.post("sendMessage", body)
	if err != nil {
		return 0, err
	}
	var msg Message
	if err := json.Unmarshal(resp.Result, &msg); err != nil {
		return 0, fmt.Errorf("parse sendMessage: %w", err)
	}
	return msg.MessageID, nil
}

func (c *Client) AnswerCallbackQuery(callbackID string, text string) error {
	body := map[string]interface{}{
		"callback_query_id": callbackID,
		"text":              text,
	}
	_, err := c.post("answerCallbackQuery", body)
	return err
}

func (c *Client) SetReactionsSupported(supported bool) {
	c.reactionsSupported = supported
}

func (c *Client) BotUserID() int64 {
	return c.botUserID
}

// --- HTTP helpers ---

func (c *Client) get(method string) (*APIResponse, error) {
	url := apiBase + c.token + "/" + method
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("telegram %s: %w", method, err)
	}
	defer resp.Body.Close()
	return c.parseResponse(method, resp.Body)
}

func (c *Client) post(method string, body interface{}) (*APIResponse, error) {
	url := apiBase + c.token + "/" + method
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("telegram %s marshal: %w", method, err)
	}
	resp, err := c.http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("telegram %s: %w", method, err)
	}
	defer resp.Body.Close()
	return c.parseResponse(method, resp.Body)
}

func (c *Client) parseResponse(method string, body io.Reader) (*APIResponse, error) {
	data, err := io.ReadAll(io.LimitReader(body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("telegram %s read: %w", method, err)
	}
	var apiResp APIResponse
	if err := json.Unmarshal(data, &apiResp); err != nil {
		return nil, fmt.Errorf("telegram %s unmarshal: %w", method, err)
	}
	if !apiResp.OK {
		return nil, fmt.Errorf("telegram %s: %s", method, apiResp.Description)
	}
	return &apiResp, nil
}
