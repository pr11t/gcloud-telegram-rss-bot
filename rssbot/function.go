package rssbot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
)

var telegramToken = os.Getenv("TELEGRAM_BOT_TOKEN")
var telegramChatID = os.Getenv("TELEGRAM_CHAT_ID")
var rssFeedURL = os.Getenv("RSS_FEED_URL")

func init() {
	if telegramToken == "" {
		panic("TELEGRAM_BOT_TOKEN environment variable empty")
	}
	if telegramChatID == "" {
		panic("TELEGRAM_CHAT_ID environment variable empty")
	}
	if rssFeedURL == "" {
		panic("RSS_FEED_URL environment variable empty")
	}
}

type PubSubMessage struct {
	Data []byte `json:"data"`
}

type RSSFeed struct {
	links []string
}

func (r *RSSFeed) fetchLinks() error {
	resp, err := http.Get(rssFeedURL)
	if err != nil {
		log.Print("Failed to fetch rss feed")
		return err
	}
	content, err := ioutil.ReadAll(resp.Body)

	r.links = make([]string, 0)
	re := regexp.MustCompile("<link>(https://[a-z0-9./-]+)</link>")
	matches := re.FindAllStringSubmatch(string(content), -1)
	for _, link := range matches {
		if len(link) == 2 {
			r.links = append(r.links, link[1])
		}
	}

	return nil
}

func (r *RSSFeed) removeOlderThan(latestNewsItem string) {
	newURLs := make([]string, 0)
	for _, url := range r.links {
		if url != latestNewsItem {
			newURLs = append(newURLs, url)
		} else {
			break
		}
	}
	r.links = newURLs
}

func (r *RSSFeed) reverse() {
	// Reverse the slice because we want to post news starting from the oldest
	for i := len(r.links)/2 - 1; i >= 0; i-- {
		opp := len(r.links) - 1 - i
		r.links[i], r.links[opp] = r.links[opp], r.links[i]
	}
}

type telegramAPI struct {
	botToken string
	chatID   string
}

type responseMessage = struct {
	OK     bool                   `json:"ok"`
	Result map[string]interface{} `json:"result"`
}

func (t *telegramAPI) call(command string, params interface{}, response interface{}) (interface{}, error) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%v/%v", t.botToken, command)
	jsonValue, err := json.Marshal(params)
	if err != nil {
		log.Panicf("Failed to marshal parameters: %v", err.Error())
	}
	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(jsonValue))
	if err != nil {
		log.Print("API call failed ", err.Error())
		return nil, err
	}
	if resp.StatusCode != 200 {
		log.Print(resp)
		return nil, errors.New("API call failed")
	}
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		log.Panicf("Failed to decode response parameters: %v", err.Error())
	}
	return response, nil
}

func (t *telegramAPI) sendMessage(text string) (interface{}, error) {
	params := struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}{
		ChatID: t.chatID,
		Text:   text,
	}
	return t.call("sendMessage", params, responseMessage{})
}

func (t *telegramAPI) setChatDescription(description string) (interface{}, error) {
	params := struct {
		ChatID      string `json:"chat_id"`
		Description string `json:"description"`
	}{
		ChatID:      t.chatID,
		Description: description,
	}
	return t.call("setChatDescription", params, responseMessage{})
}

func (t *telegramAPI) getChat() (interface{}, error) {
	params := struct {
		ChatID string `json:"chat_id"`
	}{
		ChatID: t.chatID,
	}
	return t.call("getChat", params, responseMessage{})
}

func (t *telegramAPI) getChatDescription() (description string, err error) {
	defer func() {
		if err := recover(); err != nil {
			// TODO: Handle this differently
			// If chat description is empty we can't convert interface to string
			// and program panics.
			log.Print("Chat description empty")
		}
	}()
	res, err := t.getChat()
	if err != nil {
		return
	}
	description = res.(map[string]interface{})["result"].(map[string]interface{})["description"].(string)
	return
}

func postNews(t telegramAPI, r RSSFeed) error {
	// Telegram throttles us after ~20 API calls, so just stop after this limit
	messageLimit := 10

	// Fetch news urls we want to post
	err := r.fetchLinks()
	if err != nil {
		return err
	}
	/*
		We use channel description as a value store to keep track of the last news item posted,
		because Telegram API does not allow reading bot messages by bots and there is no
		data storage backend for this program. Maybe there is a better way?
	*/
	latestPost, err := t.getChatDescription()
	if err != nil {
		log.Printf("Failed to get chat description: %v", err.Error())
		return err
	}
	r.removeOlderThan(latestPost)
	r.reverse()
	if len(r.links) < 1 {
		log.Print("No new URLs to post")
		return nil
	}
	log.Printf("%v new URLs", len(r.links))
	for _, url := range r.links {
		// Set the description first, in case something breaks down the line we may miss an article, but don't spam the channel.
		_, err := t.setChatDescription(url)
		if err != nil {
			log.Printf("Failed to set chat description: %v", err.Error())
			return err
		}
		_, err = t.sendMessage(url)
		if err != nil {
			log.Printf("Failed to send message: %v", err.Error())
			return err
		}
		// Stop when message limit is reached
		messageLimit--
		if messageLimit <= 0 {
			break
		}
	}
	return nil
}

func Run(ctx context.Context, m PubSubMessage) error {
	t := telegramAPI{botToken: telegramToken, chatID: telegramChatID}
	n := RSSFeed{}
	return postNews(t, n)
}
