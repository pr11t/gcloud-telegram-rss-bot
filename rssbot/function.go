package rssbot

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"
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
	URL string
	RSS RSS
}

type RSS struct {
	XMLName xml.Name `xml:"rss"`
	Channel Channel  `xml:"channel"`
}
type Channel struct {
	XMLName       xml.Name `xml:"channel"`
	Title         string   `xml:"title"`
	Description   string   `xml:"description"`
	Link          string   `xml:"link"`
	Language      string   `xml:"language"`
	Copyright     string   `xml:"copyright"`
	WebMaster     string   `xml:"webMaster"`
	PubDate       string   `xml:"pubDate"`
	LastBuildDate string   `xml:"lastBuildDate"`
	Generator     string   `xml:"generator"`
	Items         []Item   `xml:"item"`
}

type Item struct {
	XMLName     xml.Name `xml:"item"`
	Title       string   `xml:"title"`
	Description string   `xml:"description"`
	Link        string   `xml:"link"`
	Category    string   `xml:"category"`
}

// TelegramString returns a formated news item
func (i *Item) TelegramString() (message string) {
	if i.Category != "" {
		message += fmt.Sprintf("#%v\n", i.Category)
	}
	if i.Description != "" {
		message += fmt.Sprintf("%v\n", i.Description)
	}
	if i.Link != "" {
		message += fmt.Sprintf("%v", i.Link)
	}
	return
}

func (r *RSSFeed) Fetch() error {
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Get(r.URL)
	if err != nil {
		log.Print("Failed to fetch RSS feed")
		return err
	}
	content, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()

	err = xml.Unmarshal(content, &r.RSS)
	if err != nil {
		log.Print("Failed to unmarshal XML")
		return err
	}
	return nil
}

func (r *RSSFeed) removeOlderThan(latestNewsItem string) {
	for i := range r.RSS.Channel.Items {
		if r.RSS.Channel.Items[i].Link == latestNewsItem {
			newItems := make([]Item, i)
			copy(newItems, r.RSS.Channel.Items[0:i])
			r.RSS.Channel.Items = newItems
			return
		}
	}
}

func (r *RSSFeed) reverse() {
	// Reverse the slice because we want to post news starting from the oldest
	for i := len(r.RSS.Channel.Items)/2 - 1; i >= 0; i-- {
		opp := len(r.RSS.Channel.Items) - 1 - i
		r.RSS.Channel.Items[i], r.RSS.Channel.Items[opp] = r.RSS.Channel.Items[opp], r.RSS.Channel.Items[i]
	}
}

type TelegramAPI struct {
	APIToken string
	APIURL   string
	ChatID   string
}

func (t *TelegramAPI) call(command string, params interface{}) (response interface{}, err error) {
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	apiURL := fmt.Sprintf("%v/bot%v/%v", t.APIURL, t.APIToken, command)
	jsonValue, err := json.Marshal(params)
	if err != nil {
		log.Panicf("Failed to marshal parameters: %v", err.Error())
	}
	resp, err := client.Post(apiURL, "application/json", bytes.NewBuffer(jsonValue))
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
	err := r.Fetch()
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
	if len(r.RSS.Channel.Items) < 1 {
		log.Print("No new URLs to post")
		return nil
	}
	log.Printf("%v new URLs", len(r.RSS.Channel.Items))
	for _, item := range r.RSS.Channel.Items {
		// Set the description first, in case something breaks down the line we may miss an article, but don't spam the channel.
		if t.SetChatDescription(item.Link) != nil {
			log.Printf("Failed to set chat description: %v", err.Error())
			return err
		}
		if t.SendMessage(item.TelegramString()) != nil {
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
