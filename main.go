package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/robfig/cron/v3"
)

type Reminder struct {
	Content string    `json:"content"`
	Time    time.Time `json:"time"`
}

type UserData struct {
	Todos     []string   `json:"todos"`
	Reminders []Reminder `json:"reminders"`
}

var todoData = make(map[int64]*UserData)
var reminderScheduler = cron.New()

func parseDuration(durationStr string) (time.Duration, error) {
	unit := durationStr[len(durationStr)-1]
	value, err := strconv.Atoi(durationStr[:len(durationStr)-1])
	if err != nil {
		return 0, err
	}

	switch unit {
	case 's': // seconds
		return time.Duration(value) * time.Second, nil
	case 'm': // minutes
		return time.Duration(value) * time.Minute, nil
	case 'h': // hours
		return time.Duration(value) * time.Hour, nil
	case 'd': // days
		return time.Duration(value) * 24 * time.Hour, nil
	case 'w': // weeks
		return time.Duration(value) * 7 * 24 * time.Hour, nil
	case 'M': // months
		return time.Duration(value) * 30 * 24 * time.Hour, nil
	case 'y': // years
		return time.Duration(value) * 365 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid time unit")
	}
}

func loadUserData() error {
	file, err := os.Open("userdata.json")
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewDecoder(file).Decode(&todoData)
}

func saveUserData() error {
	file, err := os.Create("userdata.json")
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewEncoder(file).Encode(todoData)
}

func setupReminders(bot *tgbotapi.BotAPI) {
	for chatID, userData := range todoData {
		for _, reminder := range userData.Reminders {
			duration := reminder.Time.Sub(time.Now())
			if duration > 0 {
				time.AfterFunc(duration, func() {
					msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Нагадування: %s", reminder.Content))
					bot.Send(msg)
				})
			}
		}
	}
}

func main() {
	apiToken := os.Getenv("API_TOKEN")
	if apiToken == "" {
		log.Panic("API_TOKEN environment variable is not set")
	}

	bot, err := tgbotapi.NewBotAPI(apiToken)
	if err != nil {
		log.Panicf("Failed to initialize bot: %v", err)
	}

	bot.Debug = true
	log.Printf("Authorized on account %s", bot.Self.UserName)

	err = loadUserData()
	if err != nil {
		log.Printf("Failed to load user data: %v", err)
	}

	setupReminders(bot)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			handleMessage(update.Message, bot)
		}
	}
}

func handleMessage(message *tgbotapi.Message, bot *tgbotapi.BotAPI) {
	chatID := message.Chat.ID
	text := message.Text

	if strings.HasPrefix(text, "/remind") {
		parts := strings.SplitN(text, " ", 3)
		if len(parts) == 3 {
			timeStr := parts[1]
			content := parts[2]
			handleReminder(chatID, timeStr, content, bot)
		} else {
			msg := tgbotapi.NewMessage(chatID, "Usage: /remind <time> <message>")
			bot.Send(msg)
		}
	} else if strings.HasPrefix(text, "/todo") {
		handleTodoList(chatID, bot)
	} else if strings.HasPrefix(text, "/set") {
		task := strings.TrimPrefix(text, "/set ")
		handleSetTodo(chatID, task, bot)
	} else if strings.HasPrefix(text, "/done") {
		indexStr := strings.TrimPrefix(text, "/done ")
		handleMarkDone(chatID, indexStr, bot)
	} else {
		msg := tgbotapi.NewMessage(chatID, "Невідома команда!")
		bot.Send(msg)
	}
}

func handleReminder(chatID int64, timeStr string, content string, bot *tgbotapi.BotAPI) {
	duration, err := parseDuration(timeStr)
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, "Неправильний формат часу!")
		bot.Send(msg)
		return
	}

	reminderTime := time.Now().Add(duration)

	if _, exists := todoData[chatID]; !exists {
		todoData[chatID] = &UserData{Todos: []string{}, Reminders: []Reminder{}}
	}
	todoData[chatID].Reminders = append(todoData[chatID].Reminders, Reminder{
		Content: content,
		Time:    reminderTime,
	})

	time.AfterFunc(duration, func() {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Нагадування: %s", content))
		bot.Send(msg)
	})

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Ви встановили нагадування на %s від зараз!", timeStr))
	bot.Send(msg)

	if err := saveUserData(); err != nil {
		log.Printf("Failed to save user data: %v", err)
	}
}

func handleTodoList(chatID int64, bot *tgbotapi.BotAPI) {
	userData, exists := todoData[chatID]
	if !exists || len(userData.Todos) == 0 {
		msg := tgbotapi.NewMessage(chatID, "Ваш список справ порожній.")
		bot.Send(msg)
		return
	}

	var todoList string
	for i, task := range userData.Todos {
		todoList += fmt.Sprintf("%d. %s\n", i+1, task)
	}

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Список задач: \n%s", todoList))
	bot.Send(msg)
}

func handleSetTodo(chatID int64, task string, bot *tgbotapi.BotAPI) {
	if _, exists := todoData[chatID]; !exists {
		todoData[chatID] = &UserData{Todos: []string{}, Reminders: []Reminder{}}
	}
	todoData[chatID].Todos = append(todoData[chatID].Todos, task)

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Задачу '%s' додано!", task))
	bot.Send(msg)

	if err := saveUserData(); err != nil {
		log.Printf("Failed to save user data: %v", err)
	}
}

func handleMarkDone(chatID int64, indexStr string, bot *tgbotapi.BotAPI) {
	userData, exists := todoData[chatID]
	if !exists || len(userData.Todos) == 0 {
		msg := tgbotapi.NewMessage(chatID, "Ваш список справ порожній.")
		bot.Send(msg)
		return
	}

	index, err := strconv.Atoi(indexStr)
	if err != nil || index < 1 || index > len(userData.Todos) {
		msg := tgbotapi.NewMessage(chatID, "Invalid index.")
		bot.Send(msg)
		return
	}

	userData.Todos = append(userData.Todos[:index-1], userData.Todos[index:]...)
	msg := tgbotapi.NewMessage(chatID, "Виконано!")
	bot.Send(msg)

	if err := saveUserData(); err != nil {
		log.Printf("Failed to save user data: %v", err)
	}
}
