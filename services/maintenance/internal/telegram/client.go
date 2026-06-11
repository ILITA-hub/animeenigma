package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

const apiBase = "https://api.telegram.org/bot"

// Client wraps the Telegram Bot API with the methods this service needs.
type Client struct {
	token              string
	chatID             int64
	botUserID          int64
	reactionsSupported bool
	http               *http.Client
	log                *logger.Logger
}

func NewClient(token string, chatID int64, log *logger.Logger) *Client {
	return &Client{
		token:  token,
		chatID: chatID,
		http: &http.Client{
			Timeout: 70 * time.Second, // Must be > long-poll timeout (60s)
		},
		log: log,
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

	// Media — present when the message carries an attachment. Photo holds
	// multiple sizes of the same image; the last element is the largest.
	Caption      string      `json:"caption,omitempty"`
	Photo        []PhotoSize `json:"photo,omitempty"`
	Document     *Document   `json:"document,omitempty"`
	Video        *Video      `json:"video,omitempty"`
	Audio        *Audio      `json:"audio,omitempty"`
	Voice        *Voice      `json:"voice,omitempty"`
	MediaGroupID string      `json:"media_group_id,omitempty"`

	// ForwardOrigin is set on forwarded messages (Bot API 7.0+).
	ForwardOrigin *ForwardOrigin `json:"forward_origin,omitempty"`
}

type PhotoSize struct {
	FileID   string `json:"file_id"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FileSize int64  `json:"file_size,omitempty"`
}

type Document struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	FileSize int64  `json:"file_size,omitempty"`
}

type Video struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	FileSize int64  `json:"file_size,omitempty"`
	Duration int    `json:"duration,omitempty"`
}

type Audio struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	FileSize int64  `json:"file_size,omitempty"`
}

type Voice struct {
	FileID   string `json:"file_id"`
	MimeType string `json:"mime_type,omitempty"`
	FileSize int64  `json:"file_size,omitempty"`
}

// ForwardOrigin describes where a forwarded message came from. Type is one of
// "user", "hidden_user", "chat", "channel".
type ForwardOrigin struct {
	Type            string    `json:"type"`
	SenderUser      *UserInfo `json:"sender_user,omitempty"`
	SenderUserName  string    `json:"sender_user_name,omitempty"`
	SenderChat      *Chat     `json:"sender_chat,omitempty"`
	Chat            *Chat     `json:"chat,omitempty"`
	AuthorSignature string    `json:"author_signature,omitempty"`
}

// Label renders a human-readable origin ("@user", "Channel Title", …).
func (o *ForwardOrigin) Label() string {
	if o == nil {
		return ""
	}
	switch {
	case o.SenderUser != nil && o.SenderUser.Username != "":
		return "@" + o.SenderUser.Username
	case o.SenderUser != nil:
		return o.SenderUser.FirstName
	case o.SenderUserName != "":
		return o.SenderUserName
	case o.SenderChat != nil && o.SenderChat.Title != "":
		return o.SenderChat.Title
	case o.Chat != nil && o.Chat.Title != "":
		return o.Chat.Title
	default:
		return o.Type
	}
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
		c.log.Warnw("set reaction failed",
			"emoji", emoji,
			"message_id", messageID,
			"error", err,
		)
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

// File is the getFile result — FilePath feeds DownloadFile.
type File struct {
	FileID   string `json:"file_id"`
	FileSize int64  `json:"file_size,omitempty"`
	FilePath string `json:"file_path,omitempty"`
}

// maxDownloadSize caps attachment downloads. The Bot API itself refuses
// getFile for files over 20MB, so this is a defensive ceiling.
const maxDownloadSize = 25 * 1024 * 1024

// GetFile resolves a file_id to a downloadable file path.
func (c *Client) GetFile(fileID string) (*File, error) {
	resp, err := c.post("getFile", map[string]interface{}{"file_id": fileID})
	if err != nil {
		return nil, err
	}
	var f File
	if err := json.Unmarshal(resp.Result, &f); err != nil {
		return nil, fmt.Errorf("parse getFile: %w", err)
	}
	return &f, nil
}

// DownloadFile fetches the file bytes for a path returned by GetFile.
func (c *Client) DownloadFile(filePath string) ([]byte, error) {
	url := "https://api.telegram.org/file/bot" + c.token + "/" + filePath
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("telegram download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram download: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadSize+1))
	if err != nil {
		return nil, fmt.Errorf("telegram download read: %w", err)
	}
	if len(data) > maxDownloadSize {
		return nil, fmt.Errorf("telegram download: file exceeds %d bytes", maxDownloadSize)
	}
	return data, nil
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
