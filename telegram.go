package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type update struct {
	UpdateID int64    `json:"update_id"`
	Message  *message `json:"message,omitempty"`
}

type message struct {
	MessageID int64  `json:"message_id"`
	Chat      chat   `json:"chat"`
	Text      string `json:"text"`
}

type chat struct {
	ID int64 `json:"id"`
}

type telegramAPI struct {
	token string
}

func (api telegramAPI) sendMessage(chatID int64, text, parseMode string, silent bool) error {
	payload := map[string]any{
		"chat_id":              chatID,
		"text":                 text,
		"disable_notification": silent,
	}
	if parseMode != "" {
		payload["parse_mode"] = parseMode
	}

	return api.postJSON("sendMessage", payload, nil)
}

func (api telegramAPI) getUpdates(offset int64) ([]update, error) {
	payload := map[string]any{
		"offset":  offset,
		"timeout": 30,
	}

	var resp struct {
		OK     bool     `json:"ok"`
		Result []update `json:"result"`
	}
	if err := api.postJSON("getUpdates", payload, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, errors.New("telegram api error")
	}
	return resp.Result, nil
}

func (api telegramAPI) setWebhook(webhookURL, secretToken string) error {
	payload := map[string]any{
		"url": webhookURL,
	}
	if secretToken != "" {
		payload["secret_token"] = secretToken
	}
	return api.postJSON("setWebhook", payload, nil)
}

func (api telegramAPI) deleteWebhook() error {
	payload := map[string]any{
		"drop_pending_updates": true,
	}
	return api.postJSON("deleteWebhook", payload, nil)
}

func (api telegramAPI) postJSON(method string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/%s", api.token, method)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 35 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("telegram api status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	if out == nil {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(out)
}
