package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gempir/go-twitch-irc/v4"
	"github.com/pemistahl/lingua-go"
)

// Groq API request and response structures
type GroqRequest struct {
	Messages    []GroqMessage `json:"messages"`
	Model       string        `json:"model"`
	Temperature float32       `json:"temperature"`
}

type GroqMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type GroqResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func main() {
	// Get environment variables
	groqKey := os.Getenv("GROQ_API_KEY")
	twitchOAuth := os.Getenv("TWITCH_OAUTH")
	botUsername := os.Getenv("TWITCH_BOT_USERNAME")
	channel := os.Getenv("TWITCH_CHANNEL")
	languages := []lingua.Language{
		lingua.English,
		lingua.German,
	}
	// Fuchs replacer
	replacer := strings.NewReplacer(
		"fuchsgewand", "foxguy",
		"Fuchsgewand", "Foxguy",
		"FuchsGewand", "FoxGuy",
	)
	detector := lingua.NewLanguageDetectorBuilder().
		FromLanguages(languages...).
		WithMinimumRelativeDistance(0.80).
		Build()
	// Validate environment variables
	if groqKey == "" || twitchOAuth == "" || botUsername == "" || channel == "" {
		fmt.Println("Missing required environment variables:")
		fmt.Println("- GROQ_API_KEY")
		fmt.Println("- TWITCH_OAUTH")
		fmt.Println("- TWITCH_BOT_USERNAME")
		fmt.Println("- TWITCH_CHANNEL")
		return
	}

	// Create Twitch client
	client := twitch.NewClient(botUsername, twitchOAuth)

	// Handle incoming messages
	client.OnPrivateMessage(func(message twitch.PrivateMessage) {
		// Skip messages from the bot itself
		if message.User.Name == strings.ToLower(botUsername) {
			return
		}

		// Skip messages that only contain one word
		if strings.Contains(message.Message, " ") == false {
			return
		}
		message.Message = replacer.Replace(message.Message)

		// Print chat message for debugging
		fmt.Printf("[DEBUG] %s: %s\n", message.User.DisplayName, message.Message)

		// Detect language
		language, exists := detector.DetectLanguageOf(message.Message)
		if exists {
			fmt.Printf("[DEBUG] Language detected: %s\n", language.IsoCode639_1().String())
			if language.IsoCode639_1().String() == "DE" {
				// Translate German messages
				translation, err := translateText(groqKey, message.Message)
				if err != nil {
					fmt.Printf("[ERROR] Translation failed: %v\n", err)
					return
				}

				// Print translation for debugging
				fmt.Printf("[DEBUG] Translation: %s\n", translation)

				// Send translation to chat
				client.Say(channel, fmt.Sprintf("/me 🤖 @%c󠀀%s: %s", message.User.DisplayName[0], message.User.DisplayName[1:], translation))
			}
		}
	})

	// Join channel and connect
	client.Join(channel)
	fmt.Printf("Connecting to Twitch channel %s...\n", channel)
	err := client.Connect()
	if err != nil {
		panic(err)
	}
}

func translateText(apiKey, text string) (string, error) {
	requestBody := GroqRequest{
		Model:       "llama-3.1-8b-instant",
		Temperature: 0.2,
		// Reasoning:   "hidden",
		Messages: []GroqMessage{
			{
				Role:    "user",
				Content: fmt.Sprintf("Translate the following German text to English. Respond only with the translation: %s", text),
			},
		},
	}

	requestJSON, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://api.groq.com/openai/v1/chat/completions",
		bytes.NewReader(requestJSON))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status: %s", resp.Status)
	}

	var response GroqResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", err
	}

	if response.Error.Message != "" {
		return "", fmt.Errorf("API error: %s", response.Error.Message)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no translations received")
	}

	return strings.TrimSpace(response.Choices[0].Message.Content), nil
}
