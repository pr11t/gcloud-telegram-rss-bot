package main

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

type news struct {
	URLs []string
}

func (n *news) fetchURLs() error {
	resp, err := http.Get("http://uudised.err.ee/uudised_rss.php")
	if err != nil {
		log.Print("Failed to fetch rss feed")
		return err
	}
	content, err := ioutil.ReadAll(resp.Body)

	n.URLs = make([]string, 0)

	re := regexp.MustCompile("<link>(https://[a-z0-9./-]+)</link>")
	matches := re.FindAllStringSubmatch(string(content), -1)
	for _, link := range matches {
		if len(link) == 2 {
			n.URLs = append(n.URLs, link[1])
		}
	}

	return nil
}

func (n *news) removeOlderThan(latestNewsItem string) {
	newURLs := make([]string, 0)
	for _, url := range n.URLs {
		if url != latestNewsItem {
			newURLs = append(newURLs, url)
		} else {
			break
		}
	}
	n.URLs = newURLs
}

func (n *news) reverse() {
	// Reverse the slice because we want to post news starting from the oldest
	for i := len(n.URLs)/2 - 1; i >= 0; i-- {
		opp := len(n.URLs) - 1 - i
		n.URLs[i], n.URLs[opp] = n.URLs[opp], n.URLs[i]
	}
}

type telegramAPI struct {
	botToken string
	chatID   string
}

func (t *telegramAPI) call(command string, params interface{}) (result map[string]interface{}, err error) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%v/%v", t.botToken, command)
	jsonValue, err := json.Marshal(params)
	if err != nil {
		log.Panicf("Failed to marshal parameters: %v", err.Error())
	}
	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(jsonValue))
	if err != nil || resp.StatusCode != 200 {
		log.Print(resp)
		log.Print("API request failed ", err.Error())
		return
	}

	response := struct {
		OK     bool                   `json:"ok"`
		Result map[string]interface{} `json:"result"`
	}{}

	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		log.Panicf("Failed to decode response parameters: %v", err.Error())
	}

	if response.OK {
		result = response.Result
	} else {
		err = errors.New("Response not OK")
	}

	return
}

func (t *telegramAPI) sendMessage(text string) (map[string]interface{}, error) {
	params := struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}{
		ChatID: t.chatID,
		Text:   text,
	}
	return t.call("sendMessage", params)
}

func (t *telegramAPI) setChatDescription(description string) (map[string]interface{}, error) {
	params := struct {
		ChatID      string `json:"chat_id"`
		Description string `json:"description"`
	}{
		ChatID:      t.chatID,
		Description: description,
	}
	return t.call("setChatDescription", params)
}

func (t *telegramAPI) getChat() (map[string]interface{}, error) {
	params := struct {
		ChatID string `json:"chat_id"`
	}{
		ChatID: t.chatID,
	}
	return t.call("getChat", params)
}

func (t *telegramAPI) getChatDescription() (description string, err error) {
	resp, err := t.getChat()
	if err != nil {
		return
	}
	return resp["description"].(string), nil
}

func newTelegramAPI() telegramAPI {
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Panic("TELEGRAM_BOT_TOKEN env var not set")
	}
	chatID := os.Getenv("TELEGRAM_CHAT_ID")
	if chatID == "" {
		log.Panic("TELEGRAM_CHAT_ID env var not set")
	}
	return telegramAPI{botToken: token, chatID: chatID}
}

func postNews(t telegramAPI, n news) {
	// Bots can't send more than 20 messages in a minute to a group.
	// We will just exit if the limit is reached and continue during next run.
	messageLimit := 20

	// Fetch news urls we want to post
	err := n.fetchURLs()
	if err != nil {
		panic(err.Error())
	}
	/*
		We use channel description as a value store to keep track of the last news item posted,
		because Telegram API does not allow reading bot messages by bots and there is no
		data storage backend for this program. Maybe there is a better way?
	*/
	latestPost, err := t.getChatDescription()
	if err != nil {
		panic(err.Error())
	}
	n.removeOlderThan(latestPost)
	n.reverse()

	for _, url := range n.URLs {
		// Set the description first, in case something breaks down the line we might not start spamming the channel then at least
		_, err := t.setChatDescription(url)
		if err != nil {
			panic(err.Error())
		}
		_, err = t.sendMessage(url)
		if err != nil {
			panic(err.Error())
		}
		// Stop when message limit is reached
		messageLimit--
		if messageLimit <= 0 {
			break
		}
	}

}

func main() {
	t := newTelegramAPI()
	n := news{}
	postNews(t, n)
}
