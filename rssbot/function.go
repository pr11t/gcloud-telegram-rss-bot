package rssbot

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
)

func LoadConfig() (telegramToken, telegramChatID, rssFeedURL string) {
	if telegramToken = os.Getenv("TELEGRAM_BOT_TOKEN"); telegramToken == "" {
		panic("TELEGRAM_BOT_TOKEN environment variable empty")
	}
	if telegramChatID = os.Getenv("TELEGRAM_CHAT_ID"); telegramChatID == "" {
		panic("TELEGRAM_BOT_TOKEN environment variable empty")
	}
	if rssFeedURL = os.Getenv("RSS_FEED_URL"); rssFeedURL == "" {
		panic("TELEGRAM_BOT_TOKEN environment variable empty")
	}
	return
}

type RSSFeed struct {
	links []string
	URL   string
}

func (r *RSSFeed) FetchLinks() error {
	resp, err := http.Get(r.URL)
	if err != nil {
		log.Print("Failed to fetch RSS feed")
		return err
	}
	content, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()

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

type TelegramAPI struct {
	APIToken string
	APIURL   string
	ChatID   string
}

func (t *TelegramAPI) call(command string, params interface{}) (response interface{}, err error) {
	apiURL := fmt.Sprintf("%v/bot%v/%v", t.APIURL, t.APIToken, command)
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
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		log.Panicf("Failed to decode response parameters: %v", err.Error())
	}
	if !response.(map[string]interface{})["ok"].(bool) {
		return nil, errors.New("API did not return OK")
	}
	return response, nil
}

func (t *TelegramAPI) SendMessage(text string) (err error) {
	params := struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}{
		ChatID: t.ChatID,
		Text:   text,
	}
	_, err = t.call("sendMessage", params)
	return
}

func (t *TelegramAPI) SetChatDescription(description string) (err error) {
	params := struct {
		ChatID      string `json:"chat_id"`
		Description string `json:"description"`
	}{
		ChatID:      t.ChatID,
		Description: description,
	}
	resp, err := t.call("setChatDescription", params)
	if err != nil {
		return
	}
	if !resp.(map[string]interface{})["result"].(bool) {
		return errors.New("Description change returned false")
	}
	return
}

func (t *TelegramAPI) GetChat() (chat interface{}, err error) {
	params := struct {
		ChatID string `json:"chat_id"`
	}{
		ChatID: t.ChatID,
	}
	chat, err = t.call("getChat", params)
	fmt.Println(chat)
	if err != nil {
		return
	}
	if !chat.(map[string]interface{})["ok"].(bool) {
		return nil, errors.New("API responded not OK")
	}
	return chat, nil
}

func (t *TelegramAPI) GetChatDescription() (description string, err error) {
	res, err := t.GetChat()
	if err != nil {
		return
	}
	if val, ok := res.(map[string]interface{})["result"].(map[string]interface{})["description"]; ok {
		description = val.(string)
	}
	return
}

func PublishNews(t TelegramAPI, r RSSFeed) error {
	// Telegram throttles us after ~20 API calls, so just stop after this limit
	messageLimit := 10

	// Fetch news urls we want to post
	err := r.FetchLinks()
	if err != nil {
		return err
	}
	/*
		We use channel description as a value store to keep track of the last news item posted,
		because Telegram API does not allow reading bot messages by bots and there is no
		data storage backend for this program. Maybe there is a better way?
	*/
	latestPost, err := t.GetChatDescription()
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
		if t.SetChatDescription(url) != nil {
			log.Printf("Failed to set chat description: %v", err.Error())
			return err
		}
		if t.SendMessage(url) != nil {
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

func Run(http.ResponseWriter, *http.Request) {
	apiToken, chatID, feedURL := LoadConfig()
	t := TelegramAPI{APIToken: apiToken, ChatID: chatID, APIURL: "https://api.telegram.org"}
	n := RSSFeed{URL: feedURL}
	err := PublishNews(t, n)
	if err != nil {
		log.Print(err)
	}
}
